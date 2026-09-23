/**
 * HTTP API 客户端 —— MVVM 的 Service（Model 访问）层，基于 axios。
 * 职责：封装对后端 REST 接口的访问，只做数据收发与类型映射，不含任何 UI 状态。
 * 鉴权：请求拦截器自动携带 Authorization: Bearer token；401 时回调统一登出（由 useAuth 注册）。
 */
import axios, { type AxiosRequestConfig } from 'axios'
import type { AdmissionStatus, AuthenticationOptions, Bid, Candidate, CandidateAdmission, CandidateImportReport, CandidateImportRow, CandidateInfoPayload, CandidatePreferencesPayload, CandidateStatus, Department, LeftoverFinalResult, LeftoverOverview, LeftoverResult, Message, OidcConfig, OidcConfigPayload, OidcProbeResult, Permission, Role, Room, SystemPhase, SystemStatus, User, UserProfile } from '@/models'

/** 401 处理器：由 useAuth 注册（登出 + 跳登录页），避免循环依赖。 */
let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: (() => void) | null): void {
  onUnauthorized = fn
}

/** 当前 token：由 useAuth 设置。 */
let currentToken = ''
export function setAuthToken(token: string): void {
  currentToken = token
}

/** 读取当前 token（WS 连接发 auth 消息用）。 */
export function getAuthToken(): string {
  return currentToken
}

/** 统一 axios 实例：baseURL=/api，自动带 JWT，统一 401 与错误信息。 */
const client = axios.create({
  baseURL: '/api',
  headers: { 'Content-Type': 'application/json' },
})

client.interceptors.request.use((config) => {
  if (currentToken) {
    config.headers = config.headers ?? {}
    config.headers.Authorization = `Bearer ${currentToken}`
  }
  return config
})

/**
 * ApiError —— 带 HTTP 状态与响应体的请求错误。
 * 除统一的中文 message 外，保留结构化响应体（如批量导入整批被拒时的行级报告 rows），
 * 供调用方按字段消费。
 */
export class ApiError extends Error {
  readonly status: number
  readonly data: unknown

  constructor(message: string, status: number, data: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.data = data
  }
}

client.interceptors.response.use(
  (res) => res,
  (error) => {
    if (axios.isAxiosError(error) && error.response?.status === 401) {
      onUnauthorized?.()
    }
    const data = error.response?.data as { error?: string } | undefined
    const msg = data?.error ?? (axios.isAxiosError(error) ? error.message : String(error))
    return Promise.reject(new ApiError(msg, error.response?.status ?? 0, error.response?.data))
  },
)

async function request<T>(config: AxiosRequestConfig | string): Promise<T> {
  const res = await client.request<T>(typeof config === 'string' ? { url: config } : config)
  return res.data
}

function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>({ method: 'POST', url: path, data: body })
}

function put<T>(path: string, body?: unknown): Promise<T> {
  return request<T>({ method: 'PUT', url: path, data: body })
}

function patch<T>(path: string, body?: unknown): Promise<T> {
  return request<T>({ method: 'PATCH', url: path, data: body })
}

function del<T>(path: string): Promise<T> {
  return request<T>({ method: 'DELETE', url: path })
}

// ---- 会话与当前用户 ----

export const authApi = {
  /** 登录 = 创建会话（POST /sessions）。 */
  login(username: string, password: string): Promise<UserProfile & { token: string }> {
    return post('/sessions', { username, password })
  },
  me(): Promise<UserProfile> {
    return request('/me')
  },
}

// ---- 单点登录（OIDC） ----

/** OIDC 授权入口（整页跳转，非 XHR）。 */
export const OIDC_AUTHORIZATION_PATH = '/api/oidc/authorization'

/**
 * IdP 回调端点（**路径固定，界面不可编辑**）：必须与后端 `handler/http.go` 的
 * `GET /api/oidc/sessions` 路由一致。管理员在「登录认证」里只填主机，路径由本常量拼上；
 * 注册到 IdP 的 redirect_uri 必须是 `<主机><本路径>` 这一完整地址。
 */
export const OIDC_CALLBACK_PATH = '/api/oidc/sessions'

export const oidcApi = {
  /** 登录方式开关（公共接口，登录页据此渲染入口）。 */
  options(): Promise<AuthenticationOptions> {
    return request('/authentication')
  },
  config(): Promise<OidcConfig> {
    return request('/oidc/config')
  },
  updateConfig(body: OidcConfigPayload): Promise<{ ok: boolean }> {
    return put('/oidc/config', body)
  },
  probe(issuer: string): Promise<OidcProbeResult> {
    return post('/oidc/probes', { issuer })
  },
  /** 用回调带回的一次性登录码换取会话。 */
  createSession(code: string): Promise<UserProfile & { token: string }> {
    return post('/oidc/sessions', { code })
  },
}

// ---- 候选人 ----

export const candidateApi = {
  list(params?: {
    status?: CandidateStatus
    q?: string
    limit?: number
    offset?: number
  }): Promise<{ items: Candidate[] }> {
    return request({ url: '/candidates', params })
  },

  /**
   * 按筛选条件拉取【全部】候选人（逐页 limit=200 直到短页）。
   * 后端单页上限 200（更大 limit 会被打回 50）、不传默认 50——凡是要把名单
   * 当全集用的页面（名册 / 拉取池 / 预算统计）都必须走这里，否则会被静默截断。
   */
  async listAll(params?: { status?: CandidateStatus; q?: string }): Promise<{ items: Candidate[] }> {
    const PAGE_SIZE = 200
    const items: Candidate[] = []
    for (let offset = 0; ; offset += PAGE_SIZE) {
      const page = await request<{ items: Candidate[] }>({
        url: '/candidates',
        params: { ...params, limit: PAGE_SIZE, offset },
      })
      items.push(...page.items)
      if (page.items.length < PAGE_SIZE) break
    }
    return { items }
  },

  get(id: number): Promise<Candidate> {
    return request(`/candidates/${id}`)
  },

  create(body: CandidateInfoPayload): Promise<{ id: number }> {
    return post('/candidates', body)
  },

  /** 批量导入候选人（服务端单事务全或无；整批被拒时抛 ApiError，其 data.rows 为行级报告）。 */
  import(rows: CandidateImportRow[]): Promise<CandidateImportReport> {
    return post('/candidates/imports', { rows })
  },

  checkin(id: number): Promise<{ ok: boolean }> {
    return put(`/candidates/${id}/check-in`)
  },

  update(id: number, body: CandidateInfoPayload): Promise<{ ok: boolean }> {
    return put(`/candidates/${id}`, body)
  },

  /** 修改志愿与调剂（独立小权限 candidates.preferences：只覆盖这三列，不动其他资料）。 */
  updatePreferences(id: number, body: CandidatePreferencesPayload): Promise<{ ok: boolean }> {
    return patch(`/candidates/${id}/preferences`, body)
  },

  remove(id: number): Promise<{ ok: boolean }> {
    return del(`/candidates/${id}`)
  },

  resetStatus(id: number, status: CandidateStatus): Promise<{ ok: boolean }> {
    return put(`/candidates/${id}/status`, { status })
  },

  /** 候选人面试记录归档（按候选人维度，完成后仍可查；浏览任意登录）。 */
  messages(id: number): Promise<{ items: Message[] }> {
    return request(`/candidates/${id}/messages`)
  },

  /**
   * 向候选人面试记录归档补充一条消息（需 rooms.chat，与房间聊天同一权限）。
   * 不依赖房间与在场成员，故面试结档后仍可补充；返回新记录的 id。
   */
  appendMessage(id: number, content: string): Promise<{ id: number }> {
    return post(`/candidates/${id}/messages`, { content })
  },

  /** 编辑自己刚发出的记录（2 分钟内、仅发送者本人；服务端复核窗口与归属）。 */
  updateMessage(id: number, messageId: number, content: string): Promise<{ ok: boolean }> {
    return patch(`/candidates/${id}/messages/${messageId}`, { content })
  },

  /** 撤回自己刚发出的记录（2 分钟内、仅发送者本人）。 */
  removeMessage(id: number, messageId: number): Promise<{ ok: boolean }> {
    return del(`/candidates/${id}/messages/${messageId}`)
  },

  /**
   * 开关一条记录的表情回复（需 rooms.chat；PUT = 加上、DELETE = 撤回，均幂等）。
   * 表情按允许集提交（后端复核），路径段做 URL 编码。
   */
  setReaction(id: number, messageId: number, emoji: string, on: boolean): Promise<{ ok: boolean }> {
    const path = `/candidates/${id}/messages/${messageId}/reactions/${encodeURIComponent(emoji)}`
    return on ? put(path) : del(path)
  },
}

// ---- 录取决定（按部门分别记录） ----

export const admissionApi = {
  /** 列出当前用户可见的录取决定（默认本部门；browse_all 跨部门）。 */
  list(): Promise<{ items: CandidateAdmission[] }> {
    return request('/admissions')
  },
  /** 记录本部门对候选人的录取决定（需 admissions.record）；条目键为候选人。 */
  set(candidateId: number, status: AdmissionStatus): Promise<{ ok: boolean }> {
    return put(`/admissions/${candidateId}`, { status })
  },
}

// ---- 房间 ----

export const roomApi = {
  list(params?: { limit?: number; offset?: number }): Promise<{ items: Room[] }> {
    return request({ url: '/rooms', params })
  },

  /** 创建房间（name 可选，空/缺省=未命名）。 */
  create(name?: string): Promise<{ id: number }> {
    return post('/rooms', { name: name ?? '' })
  },

  /** 修改房间名（空串=清除命名）。 */
  rename(id: number, name: string): Promise<{ ok: boolean }> {
    return patch(`/rooms/${id}`, { name })
  },

  remove(id: number): Promise<{ ok: boolean }> {
    return del(`/rooms/${id}`)
  },

  /** 拉取式分配：把候选人设置为房间当前候选人（幂等）。 */
  pullCandidate(roomId: number, candidateId: number): Promise<{ ok: boolean }> {
    return put(`/rooms/${roomId}/candidate`, { candidate_id: candidateId })
  },
}

// ---- 面试官与角色管理 ----

export const userApi = {
  list(params?: { q?: string; limit?: number; offset?: number }): Promise<{ items: User[] }> {
    return request({ url: '/users', params })
  },
  create(body: { username: string; name: string; password: string; role_ids: number[]; department_id?: number | null }): Promise<{ id: number }> {
    return post('/users', body)
  },
  update(id: number, body: { username: string; name: string; role_ids: number[]; department_id?: number | null }): Promise<{ ok: boolean }> {
    return put(`/users/${id}`, body)
  },
  remove(id: number): Promise<{ ok: boolean }> {
    return del(`/users/${id}`)
  },
  resetPassword(id: number, newPassword: string): Promise<{ ok: boolean }> {
    return put(`/users/${id}/password`, { new_password: newPassword })
  },
}

export const roleApi = {
  list(): Promise<{ items: Role[] }> {
    return request('/roles')
  },
  create(body: { name: string; description: string; permissions: string[] }): Promise<{ id: number }> {
    return post('/roles', body)
  },
  update(id: number, body: { name: string; description: string; permissions: string[] }): Promise<{ ok: boolean }> {
    return put(`/roles/${id}`, body)
  },
  remove(id: number): Promise<{ ok: boolean }> {
    return del(`/roles/${id}`)
  },
}

// ---- 部门管理 ----

export interface DepartmentPayload {
  name: string
  description?: string
  expected_count?: number
}

export const departmentApi = {
  list(): Promise<{ items: Department[] }> {
    return request('/departments')
  },
  create(body: DepartmentPayload): Promise<{ id: number }> {
    return post('/departments', body)
  },
  update(id: number, body: DepartmentPayload): Promise<{ ok: boolean }> {
    return put(`/departments/${id}`, body)
  },
  remove(id: number): Promise<{ ok: boolean }> {
    return del(`/departments/${id}`)
  },
}

// ---- 系统状态（单例，部分更新） ----

export const systemStatusApi = {
  get(): Promise<SystemStatus> {
    return request('/system/status')
  },
  /** 部分更新系统状态：切换阶段（phase）与/或出价步长（bid_step）。 */
  patch(body: { phase?: SystemPhase; bid_step?: number }): Promise<{ ok: boolean }> {
    return patch('/system/status', body)
  },
}

// ---- 捡漏阶段竞拍 ----

export const leftoverApi = {
  /** 捡漏总览：当前阶段 + 各部门预算摘要（他部门出价保密为 null）+ 本部门预算。 */
  overview(): Promise<LeftoverOverview> {
    return request('/leftover')
  },

  /** 本部门当前出价列表。 */
  bids(): Promise<{ items: Bid[] }> {
    return request('/leftover/bids')
  },

  /** 设置本部门对某候选人的出价（幂等 upsert；超出剩余预算返回 budget_exceeded）。 */
  setBid(candidateId: number, amount: number): Promise<{ item: Bid }> {
    return put(`/leftover/bids/${candidateId}`, { amount })
  },

  /** 已结算赢家列表（全员可见；进入结算阶段时由后端按出价自动结算）。 */
  results(): Promise<{ items: LeftoverResult[] }> {
    return request('/leftover/results')
  },

  /** 结算预览（只读计算：赢家 = 当前最高出价部门；`resolved` 表示已正式落库）。 */
  projections(): Promise<{ items: LeftoverFinalResult[] }> {
    return request('/leftover/projections')
  },
}
