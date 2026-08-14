/**
 * 消息领域 —— 纯函数层。
 * 所有函数无副作用：输入不修改，返回新值；不依赖 Vue 响应式。
 */
import type { Message } from '@/models'
import type { ChanEvent } from '@/api/ws-model'

/** 后端 message_appended 事件 Data 字段（Go 无 tag 结构，字段名首字母大写）。 */
export interface AppendedEventData {
  RoomID?: number
  SenderID?: number
  Content?: string
}

/** 从「message_appended」频道事件构建一条 Message；非该事件返回 null。 */
export function messageFromEvent(ev: ChanEvent, fallbackRoomId: number): Message | null {
  if (ev.type !== 'message_appended') return null
  if (ev.msg_id && ev.msg_id <= 0) return null
  const d = (ev.data as AppendedEventData) ?? {}
  return {
    id: ev.msg_id ?? 0,
    room_id: d.RoomID ?? ev.room_id ?? fallbackRoomId,
    sender_id: d.SenderID ?? 0,
    content: d.Content ?? '',
    created_at: new Date().toISOString(),
  }
}

/** 判断某消息是否已存在于列表（按 id 幂等）。 */
export function hasMessage(messages: readonly Message[], id: number): boolean {
  return messages.some((m) => m.id === id)
}

/**
 * 不可变合并消息列表：以 id 去重，返回按 id 升序的新数组。输入不被修改。
 */
export function mergeMessages(
  current: readonly Message[],
  incoming: readonly Message[],
): Message[] {
  const seen = new Set(current.map((m) => m.id))
  const out = [...current]
  for (const m of incoming) {
    if (!seen.has(m.id)) {
      seen.add(m.id)
      out.push(m)
    }
  }
  return out.sort((a, b) => a.id - b.id)
}

/** 追加一条新消息（不可变），返回新数组；已存在则原样返回。 */
export function appendMessage(messages: readonly Message[], msg: Message): Message[] {
  return hasMessage(messages, msg.id) ? [...messages] : mergeMessages(messages, [msg])
}

/** 房间内当前最大消息 id（断线续传游标）；无消息为 0。 */
export function lastMessageId(messages: readonly Message[]): number {
  return messages.reduce((max, m) => (m.id > max ? m.id : max), 0)
}
