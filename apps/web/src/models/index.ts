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

/** 权限名（10 枚，RBAC 目录与后端一致）。 */
export const PERMISSIONS = {
  USERS_MANAGE: 'users.manage',
  CANDIDATES_MANAGE: 'candidates.manage',
  CANDIDATES_BROWSE_ALL: 'candidates.browse_all',
  CANDIDATES_CREATE: 'candidates.create',
  CANDIDATES_CHECKIN: 'candidates.checkin',
  CANDIDATES_ASSIGN: 'candidates.assign',
  ROOMS_VIEW: 'rooms.view',
  ROOMS_CHAT: 'rooms.chat',
  ROOMS_MOVE_PHASE: 'rooms.move_phase',
  ROOMS_MANAGE: 'rooms.manage',
} as const

export type Permission = (typeof PERMISSIONS)[keyof typeof PERMISSIONS]

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
  username: string
  name: string
  department_id?: number
  department?: Department
  created_at: string
  updated_at: string
  roles?: Role[]
}

export interface Department {
  id: number
  name: string
  description: string
  member_count: number
  created_at: string
  updated_at: string
}

/** 系统阶段：面试阶段 / 录取阶段（与后端 model.SystemPhase 一致）。 */
export type SystemPhase = 'interview' | 'admission'

export const SYSTEM_PHASES: SystemPhase[] = ['interview', 'admission']

export interface SystemStatus {
  id: number
  phase: SystemPhase
  updated_at: string
}

/** 录取决定状态（按部门分别记录）：待定 / 录取 / 放弃。 */
export type AdmissionStatus = 'pending' | 'admitted' | 'withdrawn'

export const ADMISSION_STATUSES: AdmissionStatus[] = ['pending', 'admitted', 'withdrawn']

/** 某部门对某候选人的录取决定记录。 */
export interface CandidateAdmission {
  id: number
  candidate_id: number
  department_id: number
  status: AdmissionStatus
  created_at: string
  updated_at: string
}

export interface Role {
  id: number
  name: string
  description: string
  permissions?: RolePermission[]
  created_at: string
  updated_at: string
}

export interface RolePermission {
  id: number
  role_id: number
  permission: string
}

export interface RoomMember {
  id: number
  room_id: number
  user_id: number
  user?: User
}

export interface Message {
  id: number
  candidate_id: number
  sender_id: number | null
  sender?: User
  content: string
  created_at: string
}

export interface Room {
  id: number
  candidate_id?: number
  candidate?: Candidate
  created_at: string
  updated_at: string
  members?: RoomMember[]
}

/** /api/me 与登录响应中的用户资料（含角色与权限并集）。 */
export interface UserProfile {
  user: User
  roles: string[]
  permissions: Permission[]
}
