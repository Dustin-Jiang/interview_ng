/**
 * useCandidates —— 候选人列表组合式函数（函数式 ViewModel）。
 * 组合 useAsync 完成"加载候选人"；增/签/分配等 action 继续以 promise 形式向下游组合。
 * 所有状态以只读 ref 暴露，仅通过返回的函数变更。
 */
import { computed, ref, type Ref } from 'vue'
import { candidateApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { Candidate, CandidateStatus } from '@/models'

export interface UseCandidates {
  /** 候选人列表（未加载或为空为 []）。 */
  readonly candidates: Ref<readonly Candidate[]>
  /** 状态筛选（'' 表示全部）。 */
  readonly statusFilter: Ref<CandidateStatus | ''>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  /** 执行加载（按当前 statusFilter）。 */
  load: () => Promise<Candidate[] | null>
  /** 更新筛选并重新加载。 */
  setStatusFilter: (value: CandidateStatus | '') => void
  /** 创建候选人并插入列表头部。 */
  create: (name: string, profile: string) => Promise<Candidate | null>
  /** 签到（NOT_CHECKED_IN → 已签到待分配）。 */
  checkin: (id: number) => Promise<void>
  /** 分配到房间（可给 roomId；缺省则新建）。返回房间 id。 */
  assign: (id: number, roomId?: number) => Promise<number | null>
}

export function useCandidates(): UseCandidates {
  const statusFilter = ref<CandidateStatus | ''>('')

  // loader 闭包捕获 statusFilter 当前值 —— 纯函数式地按当前条件查后端。
  const async = useAsync(() => candidateApi.list({ status: statusFilter.value || undefined }))

  const candidates = computed<readonly Candidate[]>(() => async.data.value?.items ?? [])

  async function load(): Promise<Candidate[] | null> {
    const res = await async.run()
    return res?.items ?? null
  }

  function setStatusFilter(value: CandidateStatus | ''): void {
    statusFilter.value = value
    void load()
  }

  async function create(name: string, profile: string): Promise<Candidate | null> {
    const { id } = await candidateApi.create({ name, profile: profile || undefined })
    const created = await candidateApi.get(id)
    const items = candidates.value
    // 不可变更新：返回新数组，插入头部。
    async.data.value = { items: [created, ...items] }
    return created
  }

  async function checkin(id: number): Promise<void> {
    await candidateApi.checkin(id)
    await load()
  }

  async function assign(id: number, roomId?: number): Promise<number | null> {
    const res = await candidateApi.assign(id, roomId)
    await load()
    return res.room_id
  }

  return {
    candidates,
    statusFilter,
    loading: async.loading,
    error: async.error,
    load,
    setStatusFilter,
    create,
    checkin,
    assign,
  }
}
