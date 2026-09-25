/**
 * useRooms —— 房间列表（`GET /rooms`）的**唯一共享数据源**。
 *
 * 两类读取方共用同一份数据与同一条刷新链路：
 *  - 房间列表页要全量房间（`rooms`）；
 *  - 手上只有候选人 `room_id` 的地方只要「id → 展示名」（`roomLabelOf`，解析口径统一走
 *    `domain/room.ts#roomLabel`：未命名 → 「未命名」）——候选人详情、候选人管理表、候场大屏。
 * 原先 `useRoomList` / `useRoomNames` 各发一次 `GET /rooms`、后者还自带一份事件订阅，
 * 收敛到本组合式后不存在第二个实现与第二条刷新链路。
 *
 * 拉取口径：**一次拉全**（`limit: 200` 即服务端上限，房间是少量物理记录；不传会被夹回默认 50），
 * 列表页与按 id 反查因此看的是同一份、同一个顺序。
 *
 * **不自作主张拉取**（理由同 `useDepartments`）：由读取方在自己需要的时候调用 `load()`；
 * 事件订阅按消费者计数（`useBoardChannel` 本身就是单例），房间**建 / 删 / 改名**到达即防抖重拉
 * （见 `domain/room.ts#ROOM_BOARD_EVENTS`：只有这三个事件是所有读取方都要的）。
 */
import { computed, type ComputedRef, type Ref } from 'vue'

import { roomApi } from '@/api/http'
import { ROOM_BOARD_EVENTS, roomLabel, UNNAMED_ROOM_LABEL } from '@/domain/room'
import { useAsync } from '@/composables/useAsync'
import { useBoardRefresh } from '@/composables/useBoardChannel'
import type { Room } from '@/models'

export interface UseRooms {
  /** 房间列表（未加载为 []）。 */
  readonly rooms: ComputedRef<readonly Room[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  /** 重新拉取（整份替换；失败收口在 `error`，不抛）。 */
  load: () => Promise<void>
  /** 房间 id → 展示名：无 id 返回空串；未加载/未命名返回「未命名」。 */
  roomLabelOf: (roomId?: number | null) => string
}

/** 服务端上限（`mem_store.go#ListRooms`：limit > 200 会被夹回默认 50）。 */
const ROOM_PAGE_LIMIT = 200

const list = useAsync(() => roomApi.list({ limit: ROOM_PAGE_LIMIT }))
const rooms = computed<readonly Room[]>(() => list.data.value?.items ?? [])
const names = computed<Record<number, string>>(() =>
  Object.fromEntries(rooms.value.map((r) => [r.id, roomLabel(r)])),
)

async function load(): Promise<void> {
  await list.run()
}

export function useRooms(): UseRooms {
  useBoardRefresh(ROOM_BOARD_EVENTS, () => void load())

  function roomLabelOf(roomId?: number | null): string {
    if (!roomId) return ''
    return names.value[roomId] ?? UNNAMED_ROOM_LABEL
  }

  return { rooms, loading: list.loading, error: list.error, load, roomLabelOf }
}
