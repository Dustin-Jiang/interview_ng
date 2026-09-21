/**
 * useLeftover —— 捡漏竞拍的 ViewModel 组合式函数（函数式）。
 * 自管理四份异步资源（总览 / 本部门出价 / 结算结果 / 候选人池）＋ 派生的索引映射，
 * 以及出价行内草稿、结算确认流程与阶段/权限判定；视图只做渲染与筛选展示。
 */
import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { toast } from 'vue-sonner'

import { leftoverApi, systemStatusApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import { useAuth } from '@/composables/useAuth'
import { useBoardRefresh } from '@/composables/useBoardChannel'
import { useCandidatePool } from '@/composables/useCandidatePool'
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
  readonly canManage: ComputedRef<boolean>
  readonly canBrowseAll: ComputedRef<boolean>
  readonly canBid: ComputedRef<boolean>
  // 出价
  /** 出价步长（系统状态，默认 10）。 */
  readonly bidStep: Ref<number>
  /** 出价草稿（candidate_id → 金额）。 */
  readonly drafts: Ref<Record<number, number | null>>
  readonly savingId: Ref<number | null>
  saveBid: (candidate: Candidate) => Promise<void>
  /** 结算某候选人（最高出价成交 + 重拉全页）。 */
  resolveCandidate: (target: Candidate) => Promise<void>
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

  /** 整页重拉：出价/结算事件与手动刷新共用同一组加载。 */
  async function reloadAll(): Promise<void> {
    await Promise.all([overviewAsync.run(), bidsAsync.run(), resultsAsync.run(), pool.load()])
  }

  useBoardRefresh(['leftover_bid', 'leftover_resolved'], () => void reloadAll())

  // ---- 出价步长：来自系统状态（管理面板可设，默认 10） ----
  const bidStep = ref(10)
  watch(overview, async (ov) => {
    if (ov && bidStep.value === 10) {
      try {
        const st = await systemStatusApi.get()
        bidStep.value = st.bid_step > 0 ? st.bid_step : 10
      } catch {
        /* 保底默认 10 */
      }
    }
  })

  // ---- 阶段与权限 ----
  const phase = computed(() => overview.value?.phase)
  const phaseLabel = computed(() => (phase.value ? PHASE_PRESENTATION[phase.value].label : '-'))
  const isLeftoverPhase = computed(() => phase.value === 'leftover')
  const canManage = computed(() => hasPermission(PERMISSIONS.CANDIDATES_MANAGE))
  /** 可出价：持 admissions.record + 已分配部门 + 处于捡漏阶段（与后端前置校验一致）。 */
  const canBid = computed(
    () => hasPermission(PERMISSIONS.ADMISSIONS_RECORD) && !!overview.value?.my && isLeftoverPhase.value,
  )

  // ---- 行内出价编辑：草稿按候选人 id 存数值（NumberField 绑定），出价列表每次重拉后整体重置 ----
  const drafts = ref<Record<number, number | null>>({})
  const savingId = ref<number | null>(null)

  watch(
    bidsByCandidate,
    (map) => {
      const next: Record<number, number | null> = {}
      for (const [candidateId, bid] of map) next[candidateId] = bid.amount
      drafts.value = next
    },
    { immediate: true },
  )

  async function saveBid(candidate: Candidate): Promise<void> {
    if (savingId.value !== null) return
    const amount = drafts.value[candidate.id]
    if (amount == null || !Number.isInteger(amount) || amount <= 0) {
      toast.error('出价必须为正整数')
      return
    }
    savingId.value = candidate.id
    try {
      await leftoverApi.setBid(candidate.id, amount)
      toast.success(`「${candidate.name}」出价已保存`)
      // 出价影响本部门 spent/remaining，总览一并重拉。
      await Promise.all([bidsAsync.run(), overviewAsync.run()])
    } catch (e) {
      toastError(e)
    } finally {
      savingId.value = null
    }
  }

  // ---- 结算（candidates.manage，仅捡漏阶段）：取最高出价成交 ----
  async function resolveCandidate(target: Candidate): Promise<void> {
    const r = await leftoverApi.resolve(target.id)
    toast.success(`「${target.name}」已结算：${deptName(r.department_id)} · ${r.amount}`)
    await reloadAll()
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
    canManage,
    canBrowseAll,
    canBid,
    bidStep,
    drafts,
    savingId,
    saveBid,
    resolveCandidate,
  }
}
