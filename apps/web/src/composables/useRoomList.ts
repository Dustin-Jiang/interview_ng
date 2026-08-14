/**
 * useRoomList —— 房间列表组合式函数（函数式 ViewModel）。
 * 组合 useAsync 加载房间列表；房间「当前状态」通过 domain/status 的 roomPhaseOf 推导。
 */
import { computed, type Ref } from 'vue'
import { roomApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import { roomPhaseOf } from '@/domain/status'
import type { Room } from '@/models'

export interface UseRoomList {
  readonly rooms: Ref<readonly Room[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  load: () => Promise<Room[] | null>
}

export function useRoomList(): UseRoomList {
  // 纯 loader：查后端返回数组。
  const async = useAsync(() => roomApi.list())

  const rooms = computed<readonly Room[]>(() => async.data.value?.items ?? [])

  async function load(): Promise<Room[] | null> {
    const res = await async.run()
    return res?.items ?? null
  }

  return { rooms, loading: async.loading, error: async.error, load }
}

/** 房间当前状态（= 绑定候选人状态）—— 纯函数转发，便于视图直接调用。 */
export { roomPhaseOf as roomStatus }
