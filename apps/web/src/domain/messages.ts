/**
 * 消息领域 —— 纯函数层。
 * 所有函数无副作用：输入不修改，返回新值；不依赖 Vue 响应式。
 */
import type { Message, MessageReaction } from '@/models'
import type { ChanEvent } from '@/api/ws-model'

/** 后端 message_appended 事件 Data 字段（Go 无 tag 结构，字段名首字母大写）。 */
interface AppendedEventData {
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

/**
 * 以权威快照对齐消息列表（重连后补齐「已存在消息上的改动」——编辑后的正文、别人的表情回复）：
 * 同 id 取快照内容（快照是权威），本地多出而快照里还没有的（拉取期间新到）原样保留，
 * 返回按 id 升序的新数组。输入不被修改。
 */
export function replaceMessages(
  current: readonly Message[],
  authoritative: readonly Message[],
): Message[] {
  const fresh = new Map(authoritative.map((m) => [m.id, m]))
  const out = current.map((m) => fresh.get(m.id) ?? m)
  const seen = new Set(out.map((m) => m.id))
  for (const m of authoritative) {
    if (!seen.has(m.id)) {
      seen.add(m.id)
      out.push(m)
    }
  }
  return out.sort((a, b) => a.id - b.id)
}

/** 追加一条新消息（不可变），返回新数组；已存在则原样返回。 */
export function appendMessage(messages: readonly Message[], msg: Message): Message[] {
  // mergeMessages 幂等去重：已存在则等价于原样（内容一致）。
  return mergeMessages(messages, [msg])
}

/** 面试记录的编辑/撤回窗口（毫秒）：与后端 `state.MessageModifyWindow` 同一口径。 */
export const MESSAGE_MODIFY_WINDOW_MS = 120_000

/**
 * 是否可编辑/撤回：**自己发送**、有候选人归属（写入路径需要 candidateId）且距发送不超过
 * `MESSAGE_MODIFY_WINDOW_MS`。只决定是否给出入口（右键菜单）；权威判定在服务端
 * （他人消息 → 403、超窗口 → 409），发送者已删除的消息无从归属，一律不可。
 */
export function canModifyMessage(
  message: Message,
  currentUserId: number | null,
  now: number = Date.now(),
): boolean {
  if (currentUserId == null || message.sender_id !== currentUserId) return false
  if (!message.candidate_id) return false
  const sentAt = new Date(message.created_at).getTime()
  if (!Number.isFinite(sentAt)) return false
  return now - sentAt <= MESSAGE_MODIFY_WINDOW_MS
}

/** 就地替换一条消息的正文（编辑后），不可变；未命中则原样返回。 */
export function updateMessage(messages: Message[], id: number, content: string): Message[] {
  return hasMessage(messages, id) ? messages.map((m) => (m.id === id ? { ...m, content } : m)) : messages
}

/** 移除一条消息（撤回后），不可变；未命中则原样返回。 */
export function removeMessage(messages: Message[], id: number): Message[] {
  return hasMessage(messages, id) ? messages.filter((m) => m.id !== id) : messages
}

/**
 * 允许的表情回复集合：与后端 `model.ReactionEmojis` **同一口径、同一顺序**（服务端永远复核）。
 * 按「态度 → 评价 → 关注 → 其他」分组，每 6 个一行铺进右键菜单（`grid-cols-6`）。
 * 面试记录是正式档案，故限定为语义明确的常用表情，不引入任意字符输入。
 */
export const REACTION_EMOJIS = [
  // 态度
  '👍', '👎', '❤️', '🎉', '😂', '😕',
  // 评价
  '✅', '❌', '💯', '⭐', '👏', '🔥',
  // 关注
  '🤔', '👀', '😮', '🙏', '🤝', '✨',
  // 其他
  '🤯', '😴', '🚀', '⚡', '⚠️', '😭',
] as const

/** 一个表情回复的回复人（右键菜单明细用：展示名 + 部门头衔）。 */
export interface ReactionActor {
  user_id: number
  /** 展示名（姓名，缺省回退用户名；都没有则空串，由界面兜底）。 */
  name: string
  /** 部门名（无部门为空串）。 */
  department: string
}

/** 一条消息上的表情汇总（按表情聚合的计数 + 我回没回 + 回复人明细）。 */
export interface ReactionSummary {
  emoji: string
  count: number
  /** 当前用户是否回了这个表情（决定点击是加还是撤）。 */
  mine: boolean
  /** 按回复先后排列的回复人（同一人同一表情只有一条，与后端唯一约束同语义）。 */
  reactors: ReactionActor[]
}

/**
 * 聚合某条消息的表情回复：按**调色板顺序**稳定排列（未知表情排在后面），
 * 便于列表位置不随计数变化跳动。`mine` 需要当前用户 id——事件载荷与它无关，
 * 各端按观察者自算，故这里显式传入。
 */
export function summarizeReactions(
  reactions: readonly MessageReaction[] | undefined,
  currentUserId: number | null,
): ReactionSummary[] {
  if (!reactions || reactions.length === 0) return []
  const byEmoji = new Map<string, ReactionSummary>()
  for (const r of reactions) {
    const actor: ReactionActor = {
      user_id: r.user_id,
      name: r.user?.name || r.user?.username || '',
      department: r.user?.department?.name ?? '',
    }
    const hit = byEmoji.get(r.emoji)
    if (hit) {
      hit.count += 1
      hit.mine = hit.mine || r.user_id === currentUserId
      hit.reactors.push(actor)
    } else {
      byEmoji.set(r.emoji, {
        emoji: r.emoji,
        count: 1,
        mine: r.user_id === currentUserId,
        reactors: [actor],
      })
    }
  }
  return [...byEmoji.values()].sort((a, b) => emojiRank(a.emoji) - emojiRank(b.emoji))
}

/** 表情在调色板中的位次；不在集合内的排到最后（按字面量稳定排序）。 */
function emojiRank(emoji: string): number {
  const idx = (REACTION_EMOJIS as readonly string[]).indexOf(emoji)
  return idx === -1 ? REACTION_EMOJIS.length : idx
}

/**
 * 在某条消息上加上一个表情回复（本人操作或他人事件的增量），不可变；
 * 已存在同 (人, 表情) 则原样返回（幂等，与后端唯一约束同语义）。
 * `actor` 为回复人的展示名/部门——实时事件的载荷自带（与 message_appended 带 SenderName 同理），
 * 带上它，靠事件得知的回复也能在右键菜单里显示「谁回的」。
 */
export function addReaction(
  messages: Message[],
  messageId: number,
  userId: number,
  emoji: string,
  actor?: { name?: string; department?: string },
): Message[] {
  return messages.map((m) => {
    if (m.id !== messageId) return m
    const list = m.reactions ?? []
    if (list.some((r) => r.user_id === userId && r.emoji === emoji)) return m
    return {
      ...m,
      reactions: [...list, { user_id: userId, emoji, user: minimalUser(userId, actor) }],
    }
  })
}

/** 用事件携带的展示名/部门拼一个最小 User 对象（与 `messageFromEvent` 的 sender 同一手法）。 */
function minimalUser(
  userId: number,
  actor?: { name?: string; department?: string },
): MessageReaction['user'] {
  if (!actor?.name) return undefined
  return {
    id: userId,
    username: actor.name,
    name: actor.name,
    department: actor.department ? { name: actor.department } : undefined,
    created_at: '',
    updated_at: '',
  }
}

/** 撤回某人在某条消息上的一个表情回复（只移除该 (人, 表情)），不可变。 */
export function removeReaction(
  messages: Message[],
  messageId: number,
  userId: number,
  emoji: string,
): Message[] {
  return messages.map((m) => {
    if (m.id !== messageId) return m
    const list = (m.reactions ?? []).filter((r) => !(r.user_id === userId && r.emoji === emoji))
    return list.length === (m.reactions ?? []).length ? m : { ...m, reactions: list }
  })
}

/**
 * 同一位发送者的连续消息归为同一「气泡组」（参照 shadcn `MessageGroup`/`BubbleGroup`）：
 * 组内只有第一条出头像/姓名，其余只留时间，气泡间距收紧、圆角相连。
 * 间隔上限 3 分钟——隔太久的两条即使同一人也不再视为一次连续发言。
 */
export const MESSAGE_GROUP_GAP_MS = 3 * 60_000

/**
 * 判断某条消息是否**延续**上一条（同组）：同一发送者（发送者已删即无从归属，不分组）、
 * 同一候选人、且时间间隔不超过 `gapMs`。纯函数，供列表渲染决定「出不出名头 / 用不用连通圆角」。
 */
export function continuesGroup(
  previous: Message | undefined,
  current: Message,
  gapMs: number = MESSAGE_GROUP_GAP_MS,
): boolean {
  if (!previous) return false
  if (previous.sender_id == null || previous.sender_id !== current.sender_id) return false
  if (previous.candidate_id !== current.candidate_id) return false
  const gap = new Date(current.created_at).getTime() - new Date(previous.created_at).getTime()
  return Number.isFinite(gap) && gap >= 0 && gap <= gapMs
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
