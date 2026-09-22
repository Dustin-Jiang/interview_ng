/**
 * useCandidateMessages —— 选中候选人的面试过程记录（归档回看）。
 *
 * 切换体验由三层保证（修复"每次切换先清空再请求"造成的骨架闪顿）：
 *  1. 按候选人 id 缓存：命中即刻回放旧内容（身份正确、零等待），随后后台静默复检，
 *     结果不一致才替换展示；未命中才走骨架（每个候选人至多一次）。
 *  2. 在途请求去重：缓存预取与加载共用同一 Promise，快速切换不重复发请求。
 *  3. prefetch(ids)：调用方在选中变更时预取相邻候选人的记录，使 ←/→ 连续浏览几乎全程命中。
 *  展示仍以 currentId 守卫：过期响应只写缓存、不写展示，快速切换不串档。
 */
import { ref, type Ref } from 'vue'

import { candidateApi } from '@/api/http'
import type { Message } from '@/models'

export interface UseCandidateMessages {
  readonly messages: Ref<Message[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string>
  /** 载入某候选人记录（命中缓存即显，未命中走骨架；后台静默复检）。 */
  load: (candidateId: number) => Promise<void>
  /** 静默预取若干候选人的记录（不触碰展示态；命中/在途自动去重）。 */
  prefetch: (candidateIds: number[]) => void
}

export function useCandidateMessages(): UseCandidateMessages {
  const messages = ref<Message[]>([])
  const loading = ref(false)
  const error = ref('')
  /** 当前展示对应的候选人 id（过期响应只写缓存，不写展示）。 */
  let currentId: number | null = null

  /** 已拉取记录：id → 消息数组。 */
  const cache = new Map<number, Message[]>()
  /** 在途请求：id → Promise（缓存预取与加载共用，去重）。 */
  const inflight = new Map<number, Promise<Message[]>>()

  /** 取某候选人的记录：命中在途则复用，否则发请求并写缓存。 */
  function fetchAndCache(candidateId: number): Promise<Message[]> {
    const existing = inflight.get(candidateId)
    if (existing) return existing
    const p = candidateApi
      .messages(candidateId)
      .then((res) => {
        cache.set(candidateId, res.items)
        return res.items
      })
      .finally(() => {
        if (inflight.get(candidateId) === p) inflight.delete(candidateId)
      })
    inflight.set(candidateId, p)
    return p
  }

  function prefetch(ids: number[]): void {
    for (const id of ids) {
      if (id > 0 && !cache.has(id)) void fetchAndCache(id).catch(() => {})
    }
  }

  async function load(candidateId: number): Promise<void> {
    currentId = candidateId
    error.value = ''
    const cached = cache.get(candidateId)
    if (cached) {
      // 命中：立即回放缓存（不闪骨架），后台静默复检——归档可能新增（如看板消息事件后回看）。
      messages.value = cached
      loading.value = false
      try {
        const fresh = await fetchAndCache(candidateId)
        if (currentId === candidateId) messages.value = fresh
      } catch {
        // 复检失败不打扰：缓存内容仍在展示。
      }
      return
    }
    // 未命中（深链首访等）：骨架，拉取后回填；每个候选人至多一次。
    messages.value = []
    loading.value = true
    try {
      const items = await fetchAndCache(candidateId)
      if (currentId === candidateId) messages.value = items
    } catch (e) {
      if (currentId === candidateId) error.value = (e as Error).message
    } finally {
      if (currentId === candidateId) loading.value = false
    }
  }

  return { messages, loading, error, load, prefetch }
}
