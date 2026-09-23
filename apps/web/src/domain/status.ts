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
 * 是否处于「面试进行中」（已分配 / 面试中）——房间绑定与消息通道只在此期间有意义。
 * 面试结档（已完成及其后的待录取 / 已录取）时后端解绑房间：房间快照应清空候选人，
 * 面试记录转为只读归档（与后端 `model.CandidateStatus.Interviewing` 同一判据）。
 */
export function isInterviewing(status: CandidateStatus | null): boolean {
  return status === 'ASSIGNED' || status === 'IN_PROGRESS'
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
 *  1. **面试过的在前，按面试时间升序**——「面试时间」取这场面试的时刻：在面试中用开始时刻
 *     （`interview_started_at`），已结束用结束时刻（`interview_completed_at`，后端结档时打点）；
 *  2. **没面试过的在后，按添加顺序**（`created_at` 升序，导入/新建即添加顺序）。
 * 并列时退回 `created_at`、再退回 `id`，保证顺序稳定且不重复。
 * 为什么不能用 updated_at：任何资料编辑 / 导入都会刷新它（见 `arrivalAt` 的同款理由）。
 */
export function sortRoster<
  T extends {
    id: number
    created_at: string
    interview_started_at?: string | null
    interview_completed_at?: string | null
  },
>(items: readonly T[]): T[] {
  /** 这场面试的时刻：在面试中 → 开始时刻；已结束 → 结束时刻；都没有 → Infinity（未面试组）。 */
  const interviewAt = (c: T): number =>
    Math.min(momentAt(c.interview_started_at), momentAt(c.interview_completed_at))
  const addedAt = (c: T): number => momentAt(c.created_at)
  return items
    .slice()
    .sort(
      (a, b) =>
        interviewAt(a) - interviewAt(b) ||
        addedAt(a) - addedAt(b) ||
        a.id - b.id,
    )
}

/**
 * 候场名单排序：过滤「已结束」不上屏，按状态优先级升序（**分档不跨**：正在面试仍在最上），
 * **档内按到达先后**（签到时刻升序）——已签到的各档因此都是「先来的在前」，就是叫号顺序；
 * 未签到的没有签到时刻，退回创建时间升序（原口径）。纯函数：输入不被修改。
 */
export function sortWaitingBoard<
  T extends { status: CandidateStatus; created_at: string; checked_in_at?: string | null },
>(items: readonly T[]): T[] {
  return items
    .filter((c) => c.status !== 'COMPLETED' && c.status !== 'ADMISSION_PENDING' && c.status !== 'ADMITTED')
    .slice()
    .sort(
      (a, b) =>
        WAITING_STATUS_RANK[a.status] - WAITING_STATUS_RANK[b.status] ||
        arrivalAt(a) - arrivalAt(b) ||
        a.created_at.localeCompare(b.created_at),
    )
}

/**
 * 拉取候选人列表的排序：先来后到（签到时刻升序）。
 * 与候场大屏共用同一条到达时刻口径；缺签到时刻的行（历史数据）排在有值的之后，
 * 并统一按 id 升序兜底，保证顺序稳定且无重复。纯函数：输入不被修改。
 */
export function sortByArrival<T extends { id: number; checked_in_at?: string | null }>(
  items: readonly T[],
): T[] {
  return items.slice().sort((a, b) => arrivalAt(a) - arrivalAt(b) || a.id - b.id)
}
