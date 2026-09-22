/**
 * useAdmissions —— 录取决定（按部门记录）+ 系统阶段组合式函数。
 * 供「候选人查看」页使用：录取/捡漏阶段展示本部门决定控件；
 * 持 candidates.browse_all 时展示各部门决定与部门归属。
 * 阶段读取与决定列表都在本函数内自管理（阶段变化 / 权限变化即时生效）。
 */
import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import { toast } from 'vue-sonner'

import { admissionApi, departmentApi } from '@/api/http'
import { useAuth } from '@/composables/useAuth'
import { useSystemStatus } from '@/composables/useSystemStatus'
import { PERMISSIONS, type AdmissionStatus, type Candidate, type CandidateAdmission, type SystemPhase } from '@/models'
import { ADMISSION_PRESENTATION } from '@/presenters/status'
import { toastError } from '@/lib/toast'

/** 某部门对当前候选人的录取决定（跨部门浏览行）。 */
export interface DepartmentAdmission {
  departmentId: number
  departmentName: string
  status: AdmissionStatus
}

export interface UseAdmissions {
  /** 系统当前阶段（读取失败为 null）。 */
  /** 当前系统阶段（取自 useSystemStatus 共享数据源）。 */
  readonly phase: ComputedRef<SystemPhase | null>
  /** 录取决定列表加载中（控件禁用用）。 */
  readonly loading: Ref<boolean>
  /** 是否持跨部门查看权限（candidates.browse_all）。 */
  readonly canBrowseAll: ComputedRef<boolean>
  /** 是否可记录录取决定（admissions.record）。 */
  readonly canRecord: ComputedRef<boolean>
  /** 是否展示录取控件：录取/捡漏阶段 + 有可看内容（本部门记录或跨部门权限）。 */
  readonly showControls: ComputedRef<boolean>
  /** 本部门对某候选人的决定（名册徽章用）。 */
  readonly statusOf: (candidateId: number) => AdmissionStatus | undefined
  /** 选中候选人本部门的决定（控件激活态）。 */
  readonly ownStatus: ComputedRef<AdmissionStatus | undefined>
  /** 选中候选人的他部门决定（仅跨部门浏览）。 */
  readonly othersOf: ComputedRef<DepartmentAdmission[]>
  /** 写入本部门决定并刷新列表。 */
  switchAdmission: (candidate: Candidate, status: AdmissionStatus) => Promise<void>
}

export function useAdmissions(selected: () => Candidate | null): UseAdmissions {
  const { hasPermission, user } = useAuth()

  // 阶段统一取自 useSystemStatus（模块级单例）：录取控件是否展示依赖它，勿在此另拉一份。
  const { phase } = useSystemStatus()
  const admissions = ref<CandidateAdmission[]>([])
  const loading = ref(false)
  /** 部门 id → 名称（跨部门浏览时展示决定归属）。 */
  const departmentNames = ref(new Map<number, string>())

  const canBrowseAll = computed(() => hasPermission(PERMISSIONS.CANDIDATES_BROWSE_ALL))
  const canRecord = computed(() => hasPermission(PERMISSIONS.ADMISSIONS_RECORD))

  const showControls = computed(() => {
    if (phase.value !== 'admission' && phase.value !== 'leftover') return false
    if (canBrowseAll.value) return true
    return !!user.value?.department_id
  })

  /** 本部门决定索引（candidate_id → status）。 */
  const mineByCandidate = computed(() => {
    const mine = user.value?.department_id
    const map = new Map<number, AdmissionStatus>()
    if (!mine) return map
    for (const a of admissions.value) {
      if (a.department_id === mine) map.set(a.candidate_id, a.status)
    }
    return map
  })

  /** 选中候选人的全部可见决定。 */
  const selectedAdmissions = computed(() =>
    admissions.value.filter((a) => a.candidate_id === selected()?.id),
  )

  const ownStatus = computed<AdmissionStatus | undefined>(() => {
    const c = selected()
    if (!user.value?.department_id || !c) return undefined
    return mineByCandidate.value.get(c.id)
  })

  /** 他部门决定（未记录 = 未表态 → 弃权），仅跨部门浏览时展示。 */
  const othersOf = computed<DepartmentAdmission[]>(() => {
    if (!canBrowseAll.value) return []
    const mine = user.value?.department_id
    const result: DepartmentAdmission[] = []
    for (const [departmentId, departmentName] of departmentNames.value) {
      if (departmentId === mine) continue
      const admission = selectedAdmissions.value.find((a) => a.department_id === departmentId)
      result.push({ departmentId, departmentName, status: admission?.status ?? 'withdrawn' })
    }
    return result
  })

  function statusOf(candidateId: number): AdmissionStatus | undefined {
    return mineByCandidate.value.get(candidateId)
  }

  async function loadAdmissions(): Promise<void> {
    if (!showControls.value || loading.value) return
    loading.value = true
    try {
      const res = await admissionApi.list()
      admissions.value = res.items ?? []
      if (canBrowseAll.value) {
        const depts = await departmentApi.list()
        departmentNames.value = new Map(depts.items.map((d) => [d.id, d.name]))
      }
    } catch {
      admissions.value = []
    } finally {
      loading.value = false
    }
  }

  async function switchAdmission(candidate: Candidate, status: AdmissionStatus): Promise<void> {
    if (loading.value) return
    try {
      await admissionApi.set(candidate.id, status)
      toast.success(`已将 ${candidate.name} 标记为「${ADMISSION_PRESENTATION[status].label}」`)
      await loadAdmissions()
    } catch (e) {
      toastError(e)
    }
  }

  // 进入录取/捡漏阶段后才拉取决定列表。
  watch(showControls, (on) => {
    if (on) void loadAdmissions()
  })


  return {
    phase,
    loading,
    canBrowseAll,
    canRecord,
    showControls,
    statusOf,
    ownStatus,
    othersOf,
    switchAdmission,
  }
}
