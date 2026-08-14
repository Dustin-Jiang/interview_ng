/**
 * 领域模型 —— 与后端 JSON 契约一一对应。
 * 仅表达数据形状，不含 UI/展示逻辑（MVVM 的 Model 层）。
 */

/** 候选人状态（与后端 model.CandidateStatus 一致的五档状态机）。 */
export type CandidateStatus =
  | 'NOT_CHECKED_IN'
  | 'CHECKED_IN_PENDING_ASSIGN'
  | 'ASSIGNED'
  | 'IN_PROGRESS'
  | 'COMPLETED'

export const CANDIDATE_STATUSES: CandidateStatus[] = [
  'NOT_CHECKED_IN',
  'CHECKED_IN_PENDING_ASSIGN',
  'ASSIGNED',
  'IN_PROGRESS',
  'COMPLETED',
]

export interface Candidate {
  id: number
  room_id?: number
  name: string
  profile: string
  status: CandidateStatus
  created_at: string
  updated_at: string
}

export interface User {
  id: number
  name: string
  role: string
}

export interface RoomMember {
  id: number
  room_id: number
  user_id: number
  user?: User
}

export interface Message {
  id: number
  room_id: number
  sender_id: number
  sender?: User
  content: string
  created_at: string
}

export interface Room {
  id: number
  candidate_id: number
  candidate?: Candidate
  current_interviewer_id?: number
  created_at: string
  updated_at: string
  messages?: Message[]
  members?: RoomMember[]
}
