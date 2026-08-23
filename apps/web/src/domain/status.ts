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
 * 用于候选人完成（COMPLETED）后的房间清空事件：后端已解绑候选人与 room_id，
 * 本地快照同步移除 candidate/candidate_id。
 */
export function clearRoomCandidate(room: Room): Room {
  const next: Room = { ...room }
  delete next.candidate
  delete next.candidate_id
  return next
}

/** 候场大屏的阶段列定义（未完成名单按状态机顺序分栏）。 */
export interface WaitingColumn {
  status: CandidateStatus
  candidates: { id: number; name: string; profile: string }[]
}

/**
 * 把名册按「未完成」四档分组成大屏列（COMPLETED 不上屏）。
 * 纯函数：输入不被修改；各组内按创建时间升序（叫号次序）。
 */
export function groupWaitingColumns(
  items: readonly { id: number; name: string; profile: string; status: CandidateStatus; created_at: string }[],
): WaitingColumn[] {
  const byStatus = new Map<CandidateStatus, WaitingColumn['candidates']>()
  for (const c of [...items].sort((a, b) => a.created_at.localeCompare(b.created_at))) {
    if (c.status === 'COMPLETED') continue
    let list = byStatus.get(c.status)
    if (!list) {
      list = []
      byStatus.set(c.status, list)
    }
    list.push({ id: c.id, name: c.name, profile: c.profile })
  }
  return (['NOT_CHECKED_IN', 'CHECKED_IN_PENDING_ASSIGN', 'ASSIGNED', 'IN_PROGRESS'] as const)
    .map((status) => ({ status, candidates: byStatus.get(status) ?? [] }))
}
