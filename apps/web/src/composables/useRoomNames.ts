/**
 * useRoomNames —— 房间 id → 展示名（候选人身上只有 room_id，列表页/大屏需按 id 反查名字）。
 * 自持数据：挂载时拉一次房间列表，并订阅看板通道的房间 CRUD 事件重拉（改名即时生效）；
 * 拿不到房间列表（如无 rooms.view）时退化为「未命名」，不影响页面其余部分。
 */
import { onMounted, ref, type Ref } from 'vue'
import { roomApi } from '@/api/http'
import { roomLabel, UNNAMED_ROOM_LABEL } from '@/domain/room'
import { useBoardRefresh } from '@/composables/useBoardChannel'

export interface UseRoomNames {
  /** 房间 id → 展示名（已解析：未命名即「未命名」，未加载的房间不在表内）。 */
  readonly names: Ref<Record<number, string>>
  /** 房间 id → 展示名：无 id 返回空串；未命名/未加载返回「未命名」。 */
  roomLabelOf: (roomId?: number | null) => string
}

export function useRoomNames(): UseRoomNames {
  const names = ref<Record<number, string>>({})

  async function load(): Promise<void> {
    try {
      const { items } = await roomApi.list({ limit: 200 })
      names.value = Object.fromEntries(items.map((r) => [r.id, roomLabel(r)]))
    } catch {
      // 无 rooms.view 等场景：保持空映射（调用方显示「未命名」）。
    }
  }

  useBoardRefresh(['room_created', 'room_deleted', 'room_renamed'], () => void load())
  onMounted(() => void load())

  function roomLabelOf(roomId?: number | null): string {
    if (!roomId) return ''
    return names.value[roomId] ?? UNNAMED_ROOM_LABEL
  }

  return { names, roomLabelOf }
}
