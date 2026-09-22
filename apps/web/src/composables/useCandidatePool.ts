/**
 * useCandidatePool —— 候选人池（listAll 逐页拉全，调用方按状态筛选）。
 * 「候选人查看 / 捡漏竞拍 / 系统状态预览」三处共用同一份拉取与加载态。
 */
import { computed, type ComputedRef, type Ref } from 'vue'

import { candidateApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { Candidate } from '@/models'

export interface UseCandidatePool {
  /** 全部候选人（未加载为 []）。 */
  readonly candidates: ComputedRef<readonly Candidate[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  load: () => Promise<Candidate[] | null>
}

export function useCandidatePool(): UseCandidatePool {
  const async = useAsync(() => candidateApi.listAll())
  const candidates = computed<readonly Candidate[]>(() => async.data.value?.items ?? [])

  async function load(): Promise<Candidate[] | null> {
    const res = await async.run()
    return res?.items ?? null
  }

  return { candidates, loading: async.loading, error: async.error, load }
}
