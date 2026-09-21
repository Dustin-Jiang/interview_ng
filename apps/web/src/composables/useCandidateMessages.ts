/**
 * useCandidateMessages —— 选中候选人的面试过程记录（归档回看）。
 * 每次切换候选人整体重取，并用请求序号丢弃过期响应（快速切换时不会串档）。
 */
import { ref, type Ref } from 'vue'

import { candidateApi } from '@/api/http'
import type { Message } from '@/models'

export interface UseCandidateMessages {
  readonly messages: Ref<Message[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string>
  /** 拉取某候选人的记录（切换候选人时清空旧内容）。 */
  load: (candidateId: number) => Promise<void>
}

export function useCandidateMessages(): UseCandidateMessages {
  const messages = ref<Message[]>([])
  const loading = ref(false)
  const error = ref('')
  /** 当前请求对应的候选人 id（过期响应直接丢弃）。 */
  let currentId: number | null = null

  async function load(candidateId: number): Promise<void> {
    currentId = candidateId
    messages.value = []
    error.value = ''
    loading.value = true
    try {
      const res = await candidateApi.messages(candidateId)
      if (currentId === candidateId) messages.value = res.items
    } catch (e) {
      if (currentId === candidateId) error.value = (e as Error).message
    } finally {
      if (currentId === candidateId) loading.value = false
    }
  }

  return { messages, loading, error, load }
}
