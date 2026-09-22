/**
 * 领域模型 —— 与后端 JSON 契约一一对应。
 * 仅表达数据形状，不含 UI/展示逻辑（MVVM 的 Model 层）。
 */

/** 候选人状态（与后端 model.CandidateStatus 一致的七档状态机）。 */
export type CandidateStatus =
  | 'NOT_CHECKED_IN'
  | 'CHECKED_IN_PENDING_ASSIGN'
  | 'ASSIGNED'
  | 'IN_PROGRESS'
  | 'COMPLETED'
  | 'ADMISSION_PENDING'
  | 'ADMITTED'

export const CANDIDATE_STATUSES: CandidateStatus[] = [
  'NOT_CHECKED_IN',
  'CHECKED_IN_PENDING_ASSIGN',
  'ASSIGNED',
  'IN_PROGRESS',
  'COMPLETED',
  'ADMISSION_PENDING',
  'ADMITTED',
]

/** 权限名（12 枚，RBAC 目录与后端一致）。 */
export const PERMISSIONS = {
  USERS_MANAGE: 'users.manage',
  CANDIDATES_MANAGE: 'candidates.manage',
  CANDIDATES_PREFERENCES: 'candidates.preferences',
  CANDIDATES_BROWSE_ALL: 'candidates.browse_all',
  ADMISSIONS_RECORD: 'admissions.record',
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
  /** 学号：候选人的身份键（纯数字、唯一，前导零有意义）。 */
  student_no: string
  name: string
  profile: string
  /** 第一志愿 */
  first_choice: string
  /** 第二志愿 */
  second_choice: string
  /** 是否接受调剂 */
  accept_adjust: boolean
  /** 手机号 */
  phone: string
  /** QQ 号 */
  qq: string
  /** 邮箱 */
  email: string
  status: CandidateStatus
  /** 进入「面试中」的时刻（null = 当前不在面试中）；面试计时以此为准。 */
  interview_started_at: string | null
  created_at: string
  updated_at: string
}

/**
 * 候选人资料字段全集（新建/编辑请求体，与后端 state.CandidateInfo 对应）。
 * 编辑为全量覆盖：缺省字段按零值清空。
 */
export interface CandidateInfoPayload {
  student_no: string
  name: string
  profile?: string
  first_choice?: string
  second_choice?: string
  accept_adjust?: boolean
  phone?: string
  qq?: string
  email?: string
}

/**
 * 志愿与调剂（`PATCH /api/candidates/:id/preferences` 请求体，与后端 state.CandidatePreferences 对应）。
 * 独立于资料全量编辑：三项必给（bool 无法区分缺省），只覆盖这三列。
 */
export interface CandidatePreferencesPayload {
  first_choice: string
  second_choice: string
  accept_adjust: boolean
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
  /** 预期人数（计划招聘规模）。 */
  expected_count: number
  member_count: number
  created_at: string
  updated_at: string
}

/** 系统阶段：面试阶段 / 录取阶段 / 捡漏阶段 / 结算阶段（与后端 model.SystemPhase 一致）。 */
export type SystemPhase = 'interview' | 'admission' | 'leftover' | 'settlement'

export const SYSTEM_PHASES: SystemPhase[] = ['interview', 'admission', 'leftover', 'settlement']

export interface SystemStatus {
  id: number
  phase: SystemPhase
  /** 出价步长（上下键调整报价的步进），管理员可改，默认 10。 */
  bid_step: number
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
  /** 房间名：可选别名（空=未命名，UI 回退「房间 #id」）；不要求唯一。 */
  name: string
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

// ---- 单点登录（OIDC） ----

/** 一条「组 → 角色」映射规则：对 ID token 声明求值 JMESPath，首个命中生效。 */
export interface OidcRule {
  id: number
  position: number
  expression: string
  role_id: number
}

/** 一条「组 → 部门」映射规则：命中即把账号所属部门设为 department_id（未命中则不改动）。 */
export interface OidcDeptRule {
  id: number
  position: number
  expression: string
  department_id: number
}

/** OIDC 配置（GET /oidc/config；明文客户端密钥永不下发，只回 client_secret_set）。 */
export interface OidcConfig {
  id: number
  enabled: boolean
  issuer: string
  client_id: string
  client_secret_set: boolean
  scopes: string
  redirect_url: string
  auto_provision: boolean
  role_rules: OidcRule[]
  department_rules: OidcDeptRule[]
  updated_at: string
}

/** 保存态的规则（无 id/position：顺序即数组顺序）。 */
export interface OidcRulePayload {
  expression: string
  role_id: number
}

/** 保存态的部门规则（同上）。 */
export interface OidcDeptRulePayload {
  expression: string
  department_id: number
}

/** PUT /oidc/config 请求体。 */
export interface OidcConfigPayload {
  enabled: boolean
  issuer: string
  client_id: string
  /** 省略/undefined = 保持不变；"" = 清除；非空 = 覆盖。 */
  client_secret?: string
  scopes: string
  redirect_url: string
  auto_provision: boolean
  role_rules: OidcRulePayload[]
  department_rules: OidcDeptRulePayload[]
}

/** 登录页可用的登录方式（GET /authentication，公共接口）。 */
export interface AuthenticationOptions {
  password: boolean
  oidc: { enabled: boolean }
}

/** 连通性探测结果（POST /oidc/probes）。 */
export interface OidcProbeResult {
  issuer: string
  authorization_endpoint: string
  token_endpoint: string
  jwks_uri: string
}

// ---- 捡漏阶段竞拍（leftover） ----

/** 捡漏出价记录（候选人 + 部门唯一；接口仅返回本部门的出价）。 */
export interface Bid {
  id: number
  candidate_id: number
  department_id: number
  amount: number
  created_at: string
  updated_at: string
}

/** 捡漏阶段部门预算摘要；spent/remaining 仅当前用户所在部门非 null（他部门出价保密）。 */
export interface LeftoverDepartmentBudget {
  id: number
  name: string
  expected_count: number
  admitted_count: number
  budget: number
  spent: number | null
  remaining: number | null
}

/** 当前用户所在部门的捡漏预算（用户无部门时整体为 null）。 */
export interface LeftoverMyBudget {
  department_id: number
  budget: number
  spent: number
  remaining: number
}

/** GET /leftover 响应：当前阶段（字符串）+ 各部门预算摘要 + 本部门预算。 */
export interface LeftoverOverview {
  phase: SystemPhase
  departments: LeftoverDepartmentBudget[]
  my: LeftoverMyBudget | null
}

/** 已结算的捡漏赢家记录（全员可见；进入结算阶段时由后端按出价结算落库）。 */
export interface LeftoverResult {
  candidate_id: number
  department_id: number
  amount: number
  created_at: string
}

/**
 * 结算预览（`GET /api/leftover/projections`，只读计算，不落库）：
 * 赢家 = 当前最高出价部门；`resolved` 表示该结果与已落库的录取一致（已正式结算）。
 */
export interface LeftoverFinalResult {
  candidate_id: number
  department_id: number
  amount: number
  resolved: boolean
}

// ---- 候选人批量导入（POST /candidates/imports） ----

/**
 * 导入行：解析 + JMESPath 映射后的规范化资料字段（服务端不解析表格）。
 * 与后端 state.CandidateInfo 一一对应（扁平 JSON）。
 */
export interface CandidateImportRow {
  student_no: string
  name: string
  profile: string
  first_choice: string
  second_choice: string
  accept_adjust: boolean
  phone: string
  qq: string
  email: string
}

/** 导入行级错误（整批拒绝时给出；index 为请求体中的行下标）。 */
export interface CandidateImportRowError {
  index: number
  error: string
}

/** 导入落库报告（单事务全或无：返回即整批已落库）。 */
export interface CandidateImportReport {
  created: number
  updated: number
  rows: { index: number; status: 'created' | 'updated'; candidate_id: number }[]
}
