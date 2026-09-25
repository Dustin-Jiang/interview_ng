/**
 * useRoomChat —— 单房间实时聊天组合式函数（函数式 ViewModel）。
 * 封装 WsChannel（WS Service）为响应式状态：房间 / 消息 / 连接 / 错误 / 阶段。
 * 事件/回执经 domain 纯函数转换为不可变状态；房间改名、候选人资料更新等全局事件按需局部刷新。
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
import type { CandidateStatus, Message, Room } from '@/models'
import { phaseRoom, clearRoomCandidate, isInterviewing, isInProgress } from '@/domain/status'
import {
  addReaction,
  appendMessage,
  lastMessageId,
  mergeMessages,
  messageFromEvent,
  removeMessage,
  removeReaction,
  replaceMessages,
  updateMessage,
} from '@/domain/messages'

export interface UseRoomChat {
  readonly room: Ref<Room | null>
  readonly messages: Ref<Message[]>
  readonly connected: Ref<boolean>
  readonly connecting: Ref<boolean>
  readonly error: Ref<string | null>
  /** 当前阶段（= 房间绑定候选人状态）。 */
  readonly phase: ComputedRef<CandidateStatus | null>
  /** 发送一条消息（`replyToId` = 引用的先行消息 id，可选）。 */
  sendMessage: (content: string, replyToId?: number | null) => void
  /** 推进到某阶段。 */
  movePhase: (to: CandidateStatus) => void
  /** 拉取一名候选人进房。 */
  pullCandidate: (candidateId: number) => Promise<void>
  reloadRoom: () => Promise<void>
}

export function useRoomChat(roomId: MaybeRefOrGetter<number | null>): UseRoomChat {
  const room = ref<Room | null>(null)
  const messages = ref<Message[]>([])
  const connected = ref(false)
  const connecting = ref(false)
  const error = ref<string | null>(null)

  /** 当前阶段（= 候选人状态）。 */
  const phase = computed<CandidateStatus | null>(
    () => (room.value?.candidate?.status as CandidateStatus) ?? null,
  )

  let channel: WsChannel | null = null
  /** 当前连接的房间 id（null 未连接）；用于过滤跨房间事件。 */
  let activeId: number | null = null
  const pendingReplies = new Map<string, (p: ReplyPayload) => void>()

  // ---- 事件/回执处理（经 domain 纯函数，不可变更新） ----
  function applyEvent(ev: ChanEvent): void {
    // 候场队列（拉取池）不在这里维护：它是 `useWaitingQueue` 这份共享数据源的事
    // （统一走看板通道刷新，房间页/大屏不再各自拉一份）。
    // 这里只处理「他人把候选人拉进当前房间」：本地补拉房间快照（自己拉的已在 REST 后刷新，幂等）。
    if (ev.type === 'candidate_assigned') {
      const toRoom = (ev.data as { RoomID?: number } | undefined)?.RoomID
      if (toRoom != null && toRoom === activeId) void reloadRoom()
      return
    }
    // room_renamed（全局事件，载荷带房间 id）→ 本房改名则重拉快照
    if (ev.type === 'room_renamed') {
      const rid = (ev.data as { room_id?: number } | undefined)?.room_id
      if (rid != null && rid === activeId) void reloadRoom()
      return
    }
    // candidate_updated（全局，载荷 {candidate_id}）→ 本房候选人的资料/志愿变化：重拉快照
    if (ev.type === 'candidate_updated') {
      const cid = (ev.data as { candidate_id?: number } | undefined)?.candidate_id
      if (cid != null && cid === room.value?.candidate?.id) void reloadRoom()
      return
    }
    // 其余事件仅处理本房间的（room_id=0 的全局事件无房间语义，忽略）
    if (ev.room_id !== 0 && ev.room_id !== activeId) return
    // message_appended → 追加消息（幂等去重）
    const msg = room.value ? messageFromEvent(ev) : null
    if (msg) {
      // 消息按候选人归属：只有本房候选人的消息并入本会话。
      // 归档补充（候选人查看页写入、候选人无房间时全局扇出）会送到所有看板连接，
      // 这里按候选人过滤，避免别人的记录串进本房会话。
      if (msg.candidate_id !== room.value?.candidate?.id) return
      messages.value = appendMessage(messages.value, msg)
      return
    }
    // message_updated / message_deleted → 就地改/撤（同样的候选人过滤）：
    // 编辑与撤回在归档与实时两路共用同一批记录，本房会话必须同步。
    if (ev.type === 'message_updated' || ev.type === 'message_deleted') {
      const ref = ev.data as { CandidateID?: number; MessageID?: number; Content?: string } | undefined
      if (ref?.CandidateID != null && ref.CandidateID === room.value?.candidate?.id && ref.MessageID) {
        messages.value =
          ev.type === 'message_updated'
            ? updateMessage(messages.value, ref.MessageID, ref.Content ?? '')
            : removeMessage(messages.value, ref.MessageID)
      }
      return
    }
    // message_reactions_changed → 就地加减一条表情回复（载荷与观察者无关，各端自算计数与「我回没回」）
    if (ev.type === 'message_reactions_changed') {
      const ref = ev.data as
        | {
            CandidateID?: number
            MessageID?: number
            Emoji?: string
            UserID?: number
            Added?: boolean
            UserName?: string
            UserDepartment?: string
          }
        | undefined
      if (
        ref?.CandidateID != null &&
        ref.CandidateID === room.value?.candidate?.id &&
        ref.MessageID &&
        ref.UserID != null &&
        ref.Emoji
      ) {
        messages.value = ref.Added
          ? addReaction(messages.value, ref.MessageID, ref.UserID, ref.Emoji, {
              name: ref.UserName,
              department: ref.UserDepartment,
            })
          : removeReaction(messages.value, ref.MessageID, ref.UserID, ref.Emoji)
        // 事件没带回复人展示名（旧服务端 / 该行查不到用户）时，正文与表情都还缺一块：
        // 按 REST 归档快照对齐一次，让「谁回了什么」拿回姓名（否则只剩「面试官 {id}」）。
        if (ref.Added && !ref.UserName) void resyncTranscript()
      }
      return
    }
    // room_phase_changed → 不可变更新房间阶段
    if (ev.type === 'room_phase_changed' && room.value) {
      const to = (ev.data as { To?: CandidateStatus | null })?.To
      if (to && !isInterviewing(to)) {
        // 面试结档（已完成 / 待录取 / 已录取）：后端已解绑房间（解绑后为空房，消息只读归档）；
        // 本地清空候选人，消息随候选人归档、新候选人会话从零开始。
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
    channel = new WsChannel(`/ws/rooms/${id}`, token, {
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
        // 增量 sync 只补「新消息」：已存在消息上的改动（编辑后的正文、别人的表情回复）补不回来，
        // 故重连后再按 REST 归档快照对齐一次——否则断线期间别人回的表情/改的字永远看不到，
        // 右键菜单里的「谁回了什么」也就少了一块。
        void resyncTranscript()
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
  function sendMessage(content: string, replyToId?: number | null): void {
    if (!room.value || !channel) return
    // 只有「面试中」（IN_PROGRESS）可写记录：待面试（还没点开始）与结档后都本地直接不发
    // （后端 AppendMessage 同样拒绝：interview_not_started / interview_finished）。
    if (!isInProgress(phase.value)) return
    const reqId = channel.sendMessage(content, replyToId)
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

  /** 拉取候选人进房：成功后刷新房间快照（候场队列由调用方按 useWaitingQueue 重拉，或由其事件刷新）。 */
  async function pullCandidate(candidateId: number): Promise<void> {
    const r = room.value
    if (!r) return
    await roomApi.pullCandidate(r.id, candidateId)
    await reloadRoom()
  }

  /**
   * 以 REST 归档快照对齐本地消息（重连后调用）：同 id 取权威内容（编辑后的正文、最新表情回复），
   * 快照拉取期间新到、快照里还没有的消息保留不丢。
   */
  async function resyncTranscript(): Promise<void> {
    const cid = room.value?.candidate?.id
    if (!cid) return
    try {
      const res = await candidateApi.messages(cid)
      // 期间换了候选人（房间重绑/清空）则丢弃过期快照。
      if (room.value?.candidate?.id !== cid) return
      messages.value = replaceMessages(messages.value, res.items)
    } catch {
      // 对齐失败不打扰：本地内容仍在展示，下次事件/重连再对齐。
    }
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
  }

  return {
    room,
    messages,
    connected,
    connecting,
    error,
    phase,
    sendMessage,
    movePhase,
    pullCandidate,
    reloadRoom,
  }
}
