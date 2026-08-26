/**
 * useRoomChat —— 单房间实时聊天组合式函数（函数式 ViewModel）。
 * 封装 WsChannel（WS Service）为响应式状态：房间 / 消息 / 连接 / 错误 / 阶段。
 *
 * 组合式职责（替代原 Pinia store）：
 *  - 接受一个 roomId（可响应式），watch 其变化自动切换连接；
 *  - 连接后先发 auth（JWT），鉴权成功才进入业务阶段；
 *  - 事件/回执经 domain 纯函数转换为不可变状态；
 *  - 组件卸载（onScopeDispose）自动断开连接，无需手动清理。
 */
import { computed, onScopeDispose, ref, toValue, watch, type ComputedRef, type MaybeRefOrGetter, type Ref } from 'vue'
import { candidateApi, getAuthToken, roomApi } from '@/api/http'
import { WsChannel } from '@/api/ws'
import type { ChanEvent, ReplyPayload } from '@/api/ws-model'
import { useAuth } from '@/composables/useAuth'
import type { CandidateStatus, Message, Room } from '@/models'
import { phaseRoom, clearRoomCandidate } from '@/domain/status'
import {
  appendMessage,
  lastMessageId,
  mergeMessages,
  messageFromEvent,
} from '@/domain/messages'

export interface UseRoomChat {
  readonly room: Ref<Room | null>
  readonly messages: Ref<Message[]>
  readonly connected: Ref<boolean>
  readonly connecting: Ref<boolean>
  readonly error: Ref<string | null>
  /** 当前阶段（= 房间绑定候选人状态）。 */
  readonly phase: ComputedRef<CandidateStatus | null>
  /** 可拉取候选人池（已签到待分配）。 */
  readonly pullPool: Ref<import('@/models').Candidate[]>
  readonly currentUserId: ComputedRef<number | null>
  /** 发送一条消息。 */
  sendMessage: (content: string) => void
  /** 推进到某阶段。 */
  movePhase: (to: CandidateStatus) => void
  /** 拉取一名候选人进房。 */
  pullCandidate: (candidateId: number) => Promise<void>
  reloadRoom: () => Promise<void>
}

export function useRoomChat(roomId: MaybeRefOrGetter<number | null>): UseRoomChat {
  const { currentUserId } = useAuth()
  const room = ref<Room | null>(null)
  const messages = ref<Message[]>([])
  const connected = ref(false)
  const connecting = ref(false)
  const error = ref<string | null>(null)
  const pullPool = ref<import('@/models').Candidate[]>([])

  /** 当前阶段（= 候选人状态）。 */
  const phase = computed<CandidateStatus | null>(
    () => (room.value?.candidate?.status as CandidateStatus) ?? null,
  )

  let channel: WsChannel | null = null
  /** 当前连接的房间 id（null 未连接）；用于过滤跨房间事件。 */
  let activeId: number | null = null
  const pendingReplies = new Map<string, (p: ReplyPayload) => void>()

  // 待分配池实时刷新：签到/拉走等"池变化"事件合并触发（防抖，避免事件风暴时频繁 HTTP 拉取）。
  let poolRefreshTimer: ReturnType<typeof setTimeout> | null = null
  function schedulePoolRefresh(): void {
    if (!channel) return
    if (poolRefreshTimer) clearTimeout(poolRefreshTimer)
    poolRefreshTimer = setTimeout(() => {
      poolRefreshTimer = null
      void refreshPullPool()
    }, 300)
  }

  // ---- 事件/回执处理（经 domain 纯函数，不可变更新） ----
  function applyEvent(ev: ChanEvent): void {
    // candidate_signed_in（全局）/ candidate_assigned → "待分配池"变化，刷新拉取列表
    if (ev.type === 'candidate_signed_in' || ev.type === 'candidate_assigned') {
      schedulePoolRefresh()
      // 他人把候选人拉进当前房间：本地补拉房间快照（自己拉的已在 REST 后刷新，此处幂等）
      const toRoom = (ev.data as { RoomID?: number } | undefined)?.RoomID
      if (ev.type === 'candidate_assigned' && toRoom != null && toRoom === activeId) {
        void reloadRoom()
      }
      return
    }
    // 其余事件仅处理本房间的（room_id=0 的全局事件无房间语义，忽略）
    if (ev.room_id !== 0 && ev.room_id !== activeId) return
    // message_appended → 追加消息（幂等去重）
    const msg = room.value ? messageFromEvent(ev) : null
    if (msg) {
      messages.value = appendMessage(messages.value, msg)
      return
    }
    // room_phase_changed → 不可变更新房间阶段
    if (ev.type === 'room_phase_changed' && room.value) {
      const to = (ev.data as { To?: CandidateStatus | null })?.To
      if (to === 'COMPLETED') {
        // 候选人完成：后端已自动清空房间（解绑候选人与 room_id）；
        // 消息按候选人归档保留，本地清空会话等待下一位候选人。
        room.value = clearRoomCandidate(room.value)
        messages.value = []
      } else if (to) {
        room.value = phaseRoom(room.value, to)
      }
    }
  }

  function applyReply(reqId: string, payload: ReplyPayload): void {
    const cb = pendingReplies.get(reqId)
    if (cb) {
      pendingReplies.delete(reqId)
      cb(payload)
      return
    }
    if (!payload.ok) error.value = payload.error ?? '操作失败'
  }

  function expectReply(reqId: string, cb: (p: ReplyPayload) => void): void {
    if (reqId) pendingReplies.set(reqId, cb)
  }

  // ---- 连接生命周期 ----
  function open(id: number): void {
    channel?.close()
    activeId = id
    room.value = null
    messages.value = []
    error.value = null
    connecting.value = true

    const token = getAuthToken()
    channel = new WsChannel(`/ws/room/${id}`, token, {
      onEvent: applyEvent,
      onReply: applyReply,
      onOpen: () => {
        connected.value = true
        connecting.value = false
      },
      // 断线重连成功：按续传游标补拉断线期间的房间快照与消息增量。
      onReopen: () => {
        connected.value = true
        connecting.value = false
        const c = channel
        if (!c) return
        const reqId = c.sync(lastMessageId(messages.value))
        expectReply(reqId, (p) => {
          if (p.ok && p.room) room.value = p.room
          if (p.messages && p.messages.length) {
            messages.value = mergeMessages(messages.value, p.messages)
          }
        })
      },
      onClose: () => {
        connected.value = false
      },
      onFatal: (msg) => {
        error.value = msg
        connecting.value = false
      },
    })
    channel.connect()

    // 首 / 断线同步：请求房间快照 + 增量消息（候选人维度续传）
    const c = channel
    const reqId = c.sync(0)
    expectReply(reqId, (p) => {
      if (!p.ok) {
        error.value = p.error ?? '同步失败'
        return
      }
      if (p.room) room.value = p.room
      if (p.messages && p.messages.length) {
        messages.value = mergeMessages(messages.value, p.messages)
      }
    })
  }

  function close(): void {
    channel?.close()
    channel = null
    activeId = null
    connected.value = false
    connecting.value = false
    room.value = null
    messages.value = []
    pendingReplies.clear()
    if (poolRefreshTimer) {
      clearTimeout(poolRefreshTimer)
      poolRefreshTimer = null
    }
  }

  // roomId 变化（含初始）自动连接 / 断开
  watch(
    () => toValue(roomId),
    (id) => (id != null && id > 0 ? open(id) : close()),
    { immediate: true },
  )

  // 组件作用域销毁时自动断开
  onScopeDispose(close)

  // ---- 命令 ----
  function sendMessage(content: string): void {
    if (!room.value || !channel) return
    const reqId = channel.sendMessage(content)
    expectReply(reqId, (p) => {
      if (!p.ok) error.value = p.error ?? '发送失败'
    })
  }

  function movePhase(to: CandidateStatus): void {
    if (!room.value || !channel) return
    const reqId = channel.movePhase(to)
    expectReply(reqId, (p) => {
      if (!p.ok) error.value = p.error ?? '阶段推进失败'
    })
  }

  /** 拉取候选人进房：成功后刷新房间快照与待分配池。 */
  async function pullCandidate(candidateId: number): Promise<void> {
    const r = room.value
    if (!r) return
    await roomApi.pullCandidate(r.id, candidateId)
    pullPool.value = pullPool.value.filter((c) => c.id !== candidateId)
    await reloadRoom()
  }

  /** 刷新房间快照（WS sync 拉最新），并同步待分配池。 */
  async function reloadRoom(): Promise<void> {
    const r = room.value
    if (!r || !channel) return
    const reqId = channel.sync(lastMessageId(messages.value))
    expectReply(reqId, (p) => {
      if (p.ok && p.room) room.value = p.room
      if (p.messages && p.messages.length) {
        messages.value = mergeMessages(messages.value, p.messages)
      }
    })
    await refreshPullPool()
  }

  /** 拉取"已签到待分配"候选人池（房间侧栏"拉取候选人"列表）。 */
  async function refreshPullPool(): Promise<void> {
    const res = await candidateApi.list({ status: 'CHECKED_IN_PENDING_ASSIGN' })
    pullPool.value = res.items
  }

  // 初始加载待分配池
  void refreshPullPool()

  return {
    room,
    messages,
    connected,
    connecting,
    error,
    phase,
    pullPool,
    currentUserId,
    sendMessage,
    movePhase,
    pullCandidate,
    reloadRoom,
  }
}
