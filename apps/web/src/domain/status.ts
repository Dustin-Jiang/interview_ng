/**
 * 状态机领域 —— 纯函数层。
 * 无关 Vue，无副作用；状态推进、校验都写成纯函数，便于组合与单测。
 */
import { CANDIDATE_STATUSES, type CandidateStatus, type Room } from '@/models'

/**
 * 返回某状态在五档状态机中的下一档；已是末态或无该状态则返回 null。
 * 仅作展示用（"推进到下一档"按钮），真正的合法性校验由后端完成。
 */
export function nextPhaseOf(status: CandidateStatus | null): CandidateStatus | null {
  if (!status) return null
  const idx = CANDIDATE_STATUSES.indexOf(status)
  if (idx < 0 || idx >= CANDIDATE_STATUSES.length - 1) return null
  return CANDIDATE_STATUSES[idx + 1]
}

/** 从房间快照推导当前状态（= 绑定候选人状态）；无候选人或未绑定视为空闲（null）。 */
export function roomPhaseOf(room: Room): CandidateStatus | null {
  return room.candidate?.status ?? null
}

/**
 * 是否处于「面试进行中」（已分配 / 面试中）——房间绑定只在此期间有意义。
 * 面试结档（已完成及其后的待录取 / 已录取）时后端解绑房间：房间快照应清空候选人，
 * 面试记录转为只读归档（与后端 `model.CandidateStatus.Interviewing` 同一判据）。
 */
export function isInterviewing(status: CandidateStatus | null): boolean {
  return status === 'ASSIGNED' || status === 'IN_PROGRESS'
}

/**
 * 是否处于「面试中」（IN_PROGRESS）—— 房间消息通道（面试记录）只在这档开放。
 * 「待面试」（ASSIGNED，已拉进房间但还没点开始面试）与结档后的录取档都不能写记录，
 * 与后端 `model.CandidateStatus.InProgress` 同一判据（**改一处必须改另一处**）。
 * 注意与 `isInterviewing` 的区别：后者描述「房间还绑着候选人」，用于结档解绑后的本地清理。
 */
export function isInProgress(status: CandidateStatus | null): boolean {
  return status === 'IN_PROGRESS'
}

/**
 * 不可变地更新房间快照中的候选人状态，返回新 Room（JSON 克隆，不修改入参）。
 */
export function phaseRoom(room: Room, status: CandidateStatus): Room {
  if (!room.candidate) return room
  return {
    ...room,
    candidate: { ...room.candidate, status },
  }
}

/**
 * 不可变地清空房间候选人（房间转空闲），返回新 Room（不修改入参）。
 * 用于面试结档（已完成 / 待录取 / 已录取）后的房间清空事件：后端已解绑候选人与 room_id，
 * 本地快照同步移除 candidate/candidate_id。
 */
export function clearRoomCandidate(room: Room): Room {
  const next: Room = { ...room }
  delete next.candidate
  delete next.candidate_id
  return next
}

/** 候场大屏的展示优先级：正在面试 > 等待开始 > 等待分配 > 其他（未签到）。 */
const WAITING_STATUS_RANK: Record<CandidateStatus, number> = {
  IN_PROGRESS: 0,
  ASSIGNED: 1,
  CHECKED_IN_PENDING_ASSIGN: 2,
  NOT_CHECKED_IN: 3,
  COMPLETED: 4,
  // 录取阶段状态不上屏（下方已过滤），仅为满足全档位 Record 而列出。
  ADMISSION_PENDING: 5,
  ADMITTED: 6,
}

/**
 * 到达时刻（毫秒）。判据是后端在「签到」那一刻打的 `checked_in_at`：
 * 不能用 created_at（那是导入顺序，与谁先到无关）或 updated_at（任何资料编辑/导入都会刷新，
 * 一边排队一边导入就会打乱顺序）。缺失或非法 → Infinity（未知到达排最后）。
 */
function arrivalAt(item: { checked_in_at?: string | null }): number {
  const t = item.checked_in_at ? new Date(item.checked_in_at).getTime() : NaN
  return Number.isFinite(t) ? t : Number.POSITIVE_INFINITY
}

/**
 * 时刻（毫秒）；缺失或非法 → Infinity（未知排最后）。与 `arrivalAt` 同一手法。
 */
function momentAt(value: string | null | undefined): number {
  const t = value ? new Date(value).getTime() : NaN
  return Number.isFinite(t) ? t : Number.POSITIVE_INFINITY
}

/**
 * 名册排序（候选人查看 / 捡漏页左栏共用，由 `RosterList` 统一应用）：
 *  1. **没有面试记录的在前（视作更早）**，按添加顺序（`created_at` 升序，导入/新建即添加顺序）；
 *  2. **面试过的在后，按面试时间升序**——「面试时间」取这场面试的时刻：在面试中用开始时刻
 *     （`interview_started_at`），已结束用结束时刻（`interview_completed_at`，后端结档时打点）。
 * 并列时退回 `created_at`、再退回 `id`，保证顺序稳定且不重复。
 * 为什么不能改用 `created_at` / `updated_at` 排「面试过」那一组：前者是导入顺序，
 * 后者会被任意资料编辑/导入刷新（见 `arrivalAt` 的同款理由）。
 */
export function sortRoster<
  T extends {
    id: number
    created_at: string
    interview_started_at?: string | null
    interview_completed_at?: string | null
  },
>(items: readonly T[]): T[] {
  /**
   * 这场面试的时刻：在面试中 → 开始时刻；已结束 → 结束时刻；都没有（没记录）→ -Infinity，
   * 因此「没面试记录」视作更早、排在面过的前面。
   */
  const interviewAt = (c: T): number => {
    const t = Math.min(momentAt(c.interview_started_at), momentAt(c.interview_completed_at))
    return Number.isFinite(t) ? t : Number.NEGATIVE_INFINITY
  }
  const addedAt = (c: T): number => momentAt(c.created_at)
  return items.slice().sort((a, b) => {
    const at = interviewAt(a)
    const bt = interviewAt(b)
    // 两个「没记录」都是 -Infinity，差是 NaN，所以显式比大小、不相减。
    if (at !== bt) return at < bt ? -1 : 1
    return addedAt(a) - addedAt(b) || a.id - b.id
  })
}

/**
 * 候场队列顺序（唯一口径，与后端 `mem_store.go#compareWaiting` 逐项同构——改一处必须改另一处）：
 *  1. `waiting_priority` 升序（null 排在所有显式序之后：新签到接在已固化队列的末尾，不插队）；
 *  2. `checked_in_at` 升序（缺失/非法排最后，见 `arrivalAt`）；
 *  3. `created_at` 升序（添加顺序）；
 *  4. `id` 升序（兜底，保证全序稳定且不重复——后端固化整档时用的就是这一顺序，
 *     少任何一项都可能让「固化的顺序」与「管理员看到的顺序」不一致）。
 */
export function compareWaiting(
  a: { id: number; created_at: string; checked_in_at?: string | null; waiting_priority?: number | null },
  b: { id: number; created_at: string; checked_in_at?: string | null; waiting_priority?: number | null },
): number {
  const byPriority = compareNum(waitingRank(a), waitingRank(b))
  if (byPriority !== 0) return byPriority
  const byArrival = compareNum(arrivalAt(a), arrivalAt(b))
  if (byArrival !== 0) return byArrival
  if (a.created_at !== b.created_at) return a.created_at < b.created_at ? -1 : 1
  return a.id - b.id
}

/** 数值比较（不用减法：`arrivalAt` 可能是 Infinity，Infinity - Infinity = NaN）。 */
function compareNum(a: number, b: number): number {
  if (a < b) return -1
  if (a > b) return 1
  return 0
}

/** 档内排序键：手动序号升序（null → MAX_SAFE_INTEGER，即排在所有显式序之后）。 */
function waitingRank(item: { waiting_priority?: number | null }): number {
  return item.waiting_priority ?? Number.MAX_SAFE_INTEGER
}

/**
 * 候场名单排序：过滤掉不上屏的档（已结束及其后的录取档），按状态优先级升序（**分档不跨**：
 * 正在面试仍在最上），档内按 `compareWaiting`（手动优先级 → 签到先后 → 添加顺序）。
 * 未做任何手动调序时，各档就是纯「先来后到」。纯函数：输入不被修改。
 */
export function sortWaitingBoard<
  T extends {
    id: number
    status: CandidateStatus
    created_at: string
    checked_in_at?: string | null
    waiting_priority?: number | null
  },
>(items: readonly T[]): T[] {
  return items
    .filter((c) => c.status !== 'COMPLETED' && c.status !== 'ADMISSION_PENDING' && c.status !== 'ADMITTED')
    .slice()
    .sort((a, b) => {
      const byTier = WAITING_STATUS_RANK[a.status] - WAITING_STATUS_RANK[b.status]
      return byTier !== 0 ? byTier : compareWaiting(a, b)
    })
}

/**
 * 房间「拉取候选人」池排序（候场队列口径）：与候场大屏档内同一条 `compareWaiting`，
 * RoomSidebar 据此显示 1..N 序号。纯函数：输入不被修改。
 */
export function sortWaitingPool<
  T extends {
    id: number
    status: CandidateStatus
    created_at: string
    checked_in_at?: string | null
    waiting_priority?: number | null
  },
>(items: readonly T[]): T[] {
  return items.slice().sort(compareWaiting)
}

/**
 * 名册/大屏都需要「看板通道」上这些事件触发防抖重拉——候选人签到、拉取、增删改与房间阶段变化。
 * 两处共用一份定义，避免事件清单漂移。
 */
export const ROSTER_BOARD_EVENTS = [
  'candidate_signed_in',
  'candidate_assigned',
  'candidate_created',
  'candidate_updated',
  'candidate_deleted',
  // 候场队列手动调序（顺序变化，与资料编辑分开的独立事件）。
  'candidate_priority_changed',
  'room_phase_changed',
] as const

/**
 * 候场队列（`useWaitingQueue`）关心的事件：能改变队列**成员**或**顺序**或候选人显示字段的那些。
 * 不含 `candidate_created`（新建必为未签到，进不了队列）：导入会逐行发该事件，
 * 订阅它会让队列在导入过程中反复重拉。
 * `room_phase_changed` 必须留着——把候选人重置回「已签到待分配」（或被重置走）走的就是它。
 */
// 显式标 readonly string[]：消费者拿到的是事件名（string），不是字面量联合。
export const QUEUE_BOARD_EVENTS: readonly string[] = [
  'candidate_signed_in',
  'candidate_assigned',
  'candidate_updated',
  'candidate_deleted',
  'candidate_priority_changed',
  'room_phase_changed',
] as const
