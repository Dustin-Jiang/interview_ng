/**
 * useAdmissions —— 录取决定（按部门记录）+ 系统阶段组合式函数。
 * 供「候选人查看」页使用：**面试阶段起**（面试 / 录取 / 捡漏）展示本部门决定控件（结算阶段不展示）；
 * 持 candidates.browse_all 时展示各部门决定与部门归属。
 *
 * 数据全部取自各自的唯一共享数据源（`useAdmissionList` / `useDepartments`，均为模块级单例）：
 * 本函数只管「什么时候该读」（阶段门控）与「怎么写」（本部门决定），不再自持一份 fetch。
 */
import { computed, watch, type ComputedRef, type Ref } from 'vue'
import { toast } from 'vue-sonner'

import { admissionApi } from '@/api/http'
import { isAdmissionRecordingPhase } from '@/domain/admission'
import { useAdmissionList } from '@/composables/useAdmissionList'
import { useAuth } from '@/composables/useAuth'
import { useDepartments } from '@/composables/useDepartments'
import { useSystemStatus } from '@/composables/useSystemStatus'
import { PERMISSIONS, type AdmissionStatus, type Candidate, type SystemPhase } from '@/models'
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
  /** 是否展示录取控件：面试 / 录取 / 捡漏阶段 + 有可看内容（本部门记录或跨部门权限）。 */
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
  // 决定列表与部门名单同样取自各自的唯一共享数据源（模块级单例）。
  const { admissions, loading, load: loadAdmissionList } = useAdmissionList()
  const { departments, load: loadDepartments } = useDepartments()
  /** 部门 id → 名称（跨部门浏览时展示决定归属）。 */
  const departmentNames = computed(() => new Map(departments.value.map((d) => [d.id, d.name])))

  const canBrowseAll = computed(() => hasPermission(PERMISSIONS.CANDIDATES_BROWSE_ALL))
  const canRecord = computed(() => hasPermission(PERMISSIONS.ADMISSIONS_RECORD))

  const showControls = computed(() => {
    // 阶段策略在 domain 层（面试阶段起即可表态，结算阶段不给入口），此处只叠加「有没有可看内容」。
    if (!isAdmissionRecordingPhase(phase.value)) return false
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

  /**
   * 拉取决定列表（共享源）：控件不可见时不请求（结算阶段 / 无部门且无跨部门权限）。
   * 部门名单只有跨部门浏览才用得上（决定归属要按 id 反查名字），故一并只在 canBrowseAll 时拉。
   */
  async function loadAdmissions(): Promise<void> {
    if (!showControls.value) return
    await Promise.all([
      loadAdmissionList(),
      canBrowseAll.value ? loadDepartments() : Promise.resolve(),
    ])
  }

  async function switchAdmission(candidate: Candidate, status: AdmissionStatus): Promise<void> {
    if (loading.value) return
    try {
      await admissionApi.set(candidate.id, status)
      toast.success(`已将 ${candidate.name} 标记为「${ADMISSION_PRESENTATION[status].label}」`)
      await loadAdmissionList() // 写后刷新共享源：其余读取方同一份缓存即时对齐
    } catch (e) {
      toastError(e)
    }
  }

  // 控件可见（面试阶段起）后拉取决定列表；不可见时不请求（结算阶段/无部门且无跨部门权限）。
  // `immediate` 是必需的，不是保险：`phase` 来自模块级单例 `useSystemStatus`，
  // 从别的页面切进来时它**早已加载完** → showControls 在 setup 时就已经是 true，
  // 非要等一次 false→true 才拉的话，本页永远不会请求（他人部门徽章、本部门激活态全空）；
  // 只有整页刷新（单例为空 → 阶段到达时才有一次 false→true）才碰巧正常。
  watch(
    showControls,
    (on) => {
      if (on) void loadAdmissions()
    },
    { immediate: true },
  )


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
