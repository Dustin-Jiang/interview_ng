/**
 * 消息领域 —— 纯函数层。
 * 所有函数无副作用：输入不修改，返回新值；不依赖 Vue 响应式。
 */
import type { Message } from '@/models'
import type { ChanEvent } from '@/api/ws-model'

/** 后端 message_appended 事件 Data 字段（Go 无 tag 结构，字段名首字母大写）。 */
export interface AppendedEventData {
  RoomID?: number
  CandidateID?: number
  SenderID?: number
  SenderName?: string
  /** 发送者部门名（无部门为空串）：实时消息头部的「头衔」用它，不必二次查询。 */
  SenderDepartment?: string
  Content?: string
}

/** 从「message_appended」频道事件构建一条 Message；非该事件返回 null。 */
export function messageFromEvent(ev: ChanEvent): Message | null {
  if (ev.type !== 'message_appended') return null
  if (ev.msg_id && ev.msg_id <= 0) return null
  const d = (ev.data as AppendedEventData) ?? {}
  let sender: Message['sender'] = undefined
  if (d.SenderID != null && d.SenderName) {
    // 实时事件携带发送者展示名与部门名，构造最小 sender 对象供展示「姓名 + 部门头衔」。
    sender = {
      id: d.SenderID,
      username: d.SenderName,
      name: d.SenderName,
      department: d.SenderDepartment ? { name: d.SenderDepartment } : undefined,
      created_at: '',
      updated_at: '',
    }
  }
  return {
    id: ev.msg_id ?? 0,
    candidate_id: d.CandidateID ?? 0,
    sender_id: d.SenderID ?? null,
    sender,
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

/**
 * 消息发送者展示名：自己 → 「我」；发送者已删 → 「已删除用户」；
 * 有预加载资料 → 姓名/用户名；否则退化为「面试官 {id}」（实时事件不携带 sender）。
 */
export function senderLabel(
  senderId: number | null,
  currentUserId: number | null,
  senderName?: string,
): string {
  if (senderId == null) return '已删除用户'
  if (currentUserId != null && senderId === currentUserId) return '我'
  if (senderName) return senderName
  return `面试官 ${senderId}`
}

/**
 * 发送者的部门头衔（消息头部的「谁 · 哪个部门」）。
 * 无部门（admin 等全局账号）或发送者已删除 → 空串，调用方据此不渲染头衔。
 * 数据来源两处一致：历史消息走 `sender.department`（后端预加载），实时事件走 `SenderDepartment`，
 * 都在 `messageFromEvent` 里归一到 `sender.department.name`。
 */
export function senderDepartmentLabel(m: Message): string {
  return m.sender?.department?.name ?? ''
}
