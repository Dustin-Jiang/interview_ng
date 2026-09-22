/**
 * useLeftover —— 捡漏竞拍的 ViewModel 组合式函数（函数式）。
 * 自管理四份异步资源（总览 / 本部门出价 / 结算结果 / 候选人池）＋ 派生的索引映射，
 * 以及出价行内草稿与阶段/权限判定；视图只做渲染与筛选展示。
 * 结算不在本页触发：进入「结算阶段」时后端按出价自动结算全部竞拍。
 */
import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { toast } from 'vue-sonner'

import { leftoverApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import { useAuth } from '@/composables/useAuth'
import { useBoardRefresh } from '@/composables/useBoardChannel'
import { useCandidatePool } from '@/composables/useCandidatePool'
import { useSystemStatus } from '@/composables/useSystemStatus'
import {
  PERMISSIONS,
  type Bid,
  type Candidate,
  type LeftoverOverview,
  type LeftoverResult,
} from '@/models'
import { PHASE_PRESENTATION } from '@/presenters/status'
import { toastError } from '@/lib/toast'

/** 捡漏池阶段：录取阶段的候选人（待录取 / 已录取 / 面试已结束兜底）。 */
const LEFTOVER_POOL_STAGES: Record<string, true> = {
  ADMISSION_PENDING: true,
  ADMITTED: true,
  COMPLETED: true,
}

/** 出价文本归一：只保留数字（允许空串——正在输入中）。 */
function toDigits(text: string): string {
  return text.replace(/\D+/g, '')
}

/** 把草稿文本解析为整数金额；空串/非法返回 null。 */
function parseAmount(raw?: string | null): number | null {
  if (raw == null) return null
  const text = raw.trim()
  if (!text) return null
  const n = Number(text)
  return Number.isInteger(n) && n >= 0 ? n : null
}

export interface UseLeftover {
  readonly overview: Ref<LeftoverOverview | null>
  readonly overviewError: Ref<string | null>
  /** 捡漏池候选人（按阶段过滤）。 */
  readonly candidates: ComputedRef<readonly Candidate[]>
  /** 本部门出价索引（管理员响应含全部门出价）。 */
  readonly bidsByCandidate: ComputedRef<Map<number, Bid>>
  /** 全部门出价索引（按金额降序，仅 browse_all）。 */
  readonly allBidsByCandidate: ComputedRef<Map<number, Bid[]>>
  /** 已成交结果索引。 */
  readonly resultsByCandidate: ComputedRef<Map<number, LeftoverResult>>
  /** 部门 id → 名称。 */
  readonly deptNameById: ComputedRef<Map<number, string>>
  readonly loading: ComputedRef<boolean>
  /** 候选人池自身的加载态（名册首屏骨架用）。 */
  readonly poolLoading: Ref<boolean>
  readonly error: ComputedRef<string>
  /** 部门名（缺省回退「部门#id」）。 */
  readonly deptName: (departmentId: number) => string
  reloadAll: () => Promise<void>
  // 阶段与权限
  readonly phaseLabel: ComputedRef<string>
  readonly isLeftoverPhase: ComputedRef<boolean>
  readonly canBrowseAll: ComputedRef<boolean>
  readonly canBid: ComputedRef<boolean>
  /** 出价步长（来自 useSystemStatus 共享状态，≥1，缺省 10）。 */
  readonly bidStep: ComputedRef<number>
  /** 出价草稿**文本**（candidate_id → 输入框文本；未出价预填 '0'，已出价回填金额）。
   *  草稿即输入框文本：每次输入即时同步，因此保存/步进读到的永远是屏幕上的数字。 */
  readonly drafts: Ref<Record<number, string>>
  readonly savingId: Ref<number | null>
  /** 写入草稿文本（只保留数字，允许空串）。 */
  setDraft: (candidate: Candidate, text: string) => void
  /** 按步长调整草稿（夹紧到 ≥ 0）。 */
  stepDraft: (candidate: Candidate, dir: 1 | -1) => void
  saveBid: (candidate: Candidate) => Promise<void>
}

export function useLeftover(): UseLeftover {
  const { hasPermission } = useAuth()

  // ---- 数据加载：总览 / 本部门出价 / 结算结果 / 候选人池（各自独立异步资源） ----
  const overviewAsync = useAsync(() => leftoverApi.overview())
  const bidsAsync = useAsync(() => leftoverApi.bids())
  const resultsAsync = useAsync(() => leftoverApi.results())
  const pool = useCandidatePool()

  const overview = computed(() => overviewAsync.data.value)
  const candidates = computed<readonly Candidate[]>(() =>
    pool.candidates.value.filter((c) => LEFTOVER_POOL_STAGES[c.status] === true),
  )
  const canBrowseAll = computed(() => hasPermission(PERMISSIONS.CANDIDATES_BROWSE_ALL))

  /** 出价按候选人索引：管理员响应含全部门出价，编辑列仅取本部门（无部门则空）。 */
  const bidsByCandidate = computed(() => {
    const items = bidsAsync.data.value?.items ?? []
    const mine = canBrowseAll.value
      ? items.filter((b) => b.department_id === overview.value?.my?.department_id)
      : items
    return new Map(mine.map((b) => [b.candidate_id, b]))
  })

  /** 管理端视图：候选人 → 全部门出价（按金额降序）。 */
  const allBidsByCandidate = computed(() => {
    const map = new Map<number, Bid[]>()
    for (const b of bidsAsync.data.value?.items ?? []) {
      const list = map.get(b.candidate_id) ?? []
      list.push(b)
      map.set(b.candidate_id, list)
    }
    for (const list of map.values()) list.sort((x, y) => y.amount - x.amount)
    return map
  })

  /** 已成交结果按候选人索引（唯一录取确定才返回）。 */
  const resultsByCandidate = computed(
    () => new Map((resultsAsync.data.value?.items ?? []).map((r) => [r.candidate_id, r])),
  )

  /** 部门 id → 名称（结算结果用；部门名单来自总览接口）。 */
  const deptNameById = computed(
    () => new Map((overview.value?.departments ?? []).map((d) => [d.id, d.name])),
  )

  const loading = computed(
    () => bidsAsync.loading.value || resultsAsync.loading.value || pool.loading.value,
  )
  const error = computed(() => pool.error.value || overviewAsync.error.value || '')

  function deptName(departmentId: number): string {
    return deptNameById.value.get(departmentId) ?? `部门#${departmentId}`
  }

  /** 整页重拉：出价/结算事件与手动刷新共用同一组加载（含系统状态，阶段可能刚被管理员切换）。 */
  async function reloadAll(): Promise<void> {
    await Promise.all([
      loadSystemStatus(),
      overviewAsync.run(),
      bidsAsync.run(),
      resultsAsync.run(),
      pool.load(),
    ])
  }

  /** 出价影响的资源：本部门出价 + 预算总览（阶段/结算结果/候选人池与出价无关，不重拉）。 */
  async function refreshBids(): Promise<void> {
    await Promise.all([bidsAsync.run(), overviewAsync.run()])
  }

  useBoardRefresh(['leftover_bid', 'leftover_resolved'], () => void reloadAll())

  // ---- 阶段与出价步长：统一取自 useSystemStatus（单例数据源，勿在此另拉一份） ----
  const { phase, bidStep, load: loadSystemStatus } = useSystemStatus()
  const phaseLabel = computed(() => (phase.value ? PHASE_PRESENTATION[phase.value].label : '-'))
  const isLeftoverPhase = computed(() => phase.value === 'leftover')
  /** 可出价：持 admissions.record + 已分配部门 + 处于捡漏阶段（与后端前置校验一致）。 */
  const canBid = computed(
    () => hasPermission(PERMISSIONS.ADMISSIONS_RECORD) && !!overview.value?.my && isLeftoverPhase.value,
  )

  // ---- 行内出价编辑：草稿存**输入框文本**，出价列表每次重拉后整体重置 ----
  // 未出价预填 '0'（0 是合法出价）；已出价回填实际金额。
  const drafts = ref<Record<number, string>>({})
  const savingId = ref<number | null>(null)

  watch(
    [bidsByCandidate, candidates],
    ([map, list]) => {
      const next: Record<number, string> = {}
      for (const c of list) next[c.id] = '0'
      for (const [candidateId, bid] of map) next[candidateId] = String(bid.amount)
      drafts.value = next
    },
    { immediate: true },
  )

  /** 写入草稿文本（只留数字；空串表示正在输入）。 */
  function setDraft(candidate: Candidate, text: string): void {
    drafts.value[candidate.id] = toDigits(text)
  }

  /** 按步长调整草稿（夹紧到 ≥ 0）；空/非法文本以 0 为起点。 */
  function stepDraft(candidate: Candidate, dir: 1 | -1): void {
    const current = parseAmount(drafts.value[candidate.id]) ?? 0
    drafts.value[candidate.id] = String(Math.max(0, current + dir * bidStep.value))
  }

  /** 保存出价：草稿即输入框文本，直接解析落库（空/非法 → 提示）。 */
  async function saveBid(candidate: Candidate): Promise<void> {
    if (savingId.value !== null) return
    const amount = parseAmount(drafts.value[candidate.id])
    if (amount == null) {
      toast.error('出价必须为不小于 0 的整数')
      return
    }
    savingId.value = candidate.id
    try {
      await leftoverApi.setBid(candidate.id, amount)
      toast.success(`「${candidate.name}」出价已保存`)
      await refreshBids()
    } catch (e) {
      toastError(e)
    } finally {
      savingId.value = null
    }
  }

  return {
    overview,
    overviewError: overviewAsync.error,
    candidates,
    bidsByCandidate,
    allBidsByCandidate,
    resultsByCandidate,
    deptNameById,
    loading,
    poolLoading: pool.loading,
    error,
    deptName,
    reloadAll,
    phaseLabel,
    isLeftoverPhase,
    canBrowseAll,
    canBid,
    bidStep,
    drafts,
    savingId,
    setDraft,
    stepDraft,
    saveBid,
  }
}
