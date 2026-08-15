/**
 * useCandidates —— 候选人列表组合式函数（函数式 ViewModel）。
 * 组合 useAsync 完成"加载候选人"；增/签/编辑/删除/重置等 action 以 promise 形式向下游组合。
 * 分配已改为"房间内拉取"（POST /api/rooms/:id/pull_candidate），本组合式不再提供 assign。
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
  /** 关键词搜索（姓名/简介）。 */
  readonly keyword: Ref<string>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  /** 执行加载（按当前 statusFilter + keyword）。 */
  load: () => Promise<Candidate[] | null>
  /** 更新筛选并重新加载。 */
  setStatusFilter: (value: CandidateStatus | '') => void
  /** 更新关键词并重新加载。 */
  setKeyword: (value: string) => void
  /** 创建候选人并插入列表头部。 */
  create: (name: string, profile: string) => Promise<Candidate | null>
  /** 签到（NOT_CHECKED_IN → 已签到待分配）。 */
  checkin: (id: number) => Promise<void>
  /** 编辑姓名/简介。 */
  update: (id: number, name: string, profile: string) => Promise<void>
  /** 删除候选人（级联删消息、解绑房间）。 */
  remove: (id: number) => Promise<void>
  /** 重置状态到任意档（表单约束在视图层，后端校验为准）。 */
  resetStatus: (id: number, status: CandidateStatus) => Promise<void>
}

export function useCandidates(): UseCandidates {
  const statusFilter = ref<CandidateStatus | ''>('')
  const keyword = ref('')

  // loader 闭包捕获筛选条件 —— 纯函数式地按当前条件查后端。
  const async = useAsync(() =>
    candidateApi.list({ status: statusFilter.value || undefined, q: keyword.value || undefined }),
  )

  const candidates = computed<readonly Candidate[]>(() => async.data.value?.items ?? [])

  async function load(): Promise<Candidate[] | null> {
    const res = await async.run()
    return res?.items ?? null
  }

  function setStatusFilter(value: CandidateStatus | ''): void {
    statusFilter.value = value
    void load()
  }

  function setKeyword(value: string): void {
    keyword.value = value
    void load()
  }

  async function create(name: string, profile: string): Promise<Candidate | null> {
    const { id } = await candidateApi.create({ name, profile: profile || undefined })
    const created = await candidateApi.get(id)
    // 不可变更新：返回新数组，插入头部。
    async.data.value = { items: [created, ...candidates.value] }
    return created
  }

  async function checkin(id: number): Promise<void> {
    await candidateApi.checkin(id)
    await load()
  }

  async function update(id: number, name: string, profile: string): Promise<void> {
    await candidateApi.update(id, { name, profile: profile || undefined })
    await load()
  }

  async function remove(id: number): Promise<void> {
    await candidateApi.remove(id)
    await load()
  }

  async function resetStatus(id: number, status: CandidateStatus): Promise<void> {
    await candidateApi.resetStatus(id, status)
    await load()
  }

  return {
    candidates,
    statusFilter,
    keyword,
    loading: async.loading,
    error: async.error,
    load,
    setStatusFilter,
    setKeyword,
    create,
    checkin,
    update,
    remove,
    resetStatus,
  }
}
