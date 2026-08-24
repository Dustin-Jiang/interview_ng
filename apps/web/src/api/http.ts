/**
 * HTTP API 客户端 —— MVVM 的 Service（Model 访问）层，基于 axios。
 * 职责：封装对后端 REST 接口的访问，只做数据收发与类型映射，不含任何 UI 状态。
 * 鉴权：请求拦截器自动携带 Authorization: Bearer token；401 时回调统一登出（由 useAuth 注册）。
 */
import axios, { type AxiosRequestConfig } from 'axios'
import type { Candidate, CandidateStatus, Department, Message, Permission, Role, Room, User, UserProfile } from '@/models'

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

client.interceptors.response.use(
  (res) => res,
  (error) => {
    if (axios.isAxiosError(error) && error.response?.status === 401) {
      onUnauthorized?.()
    }
    const data = error.response?.data as { error?: string } | undefined
    const msg = data?.error ?? (axios.isAxiosError(error) ? error.message : String(error))
    return Promise.reject(new Error(msg))
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

function del<T>(path: string): Promise<T> {
  return request<T>({ method: 'DELETE', url: path })
}

// ---- 认证 ----

export const authApi = {
  login(username: string, password: string): Promise<UserProfile & { token: string }> {
    return post('/auth/login', { username, password })
  },
  me(): Promise<UserProfile> {
    return request('/me')
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

  get(id: number): Promise<Candidate> {
    return request(`/candidates/${id}`)
  },

  create(body: { name: string; profile?: string }): Promise<{ id: number }> {
    return post('/candidates', body)
  },

  checkin(id: number): Promise<{ ok: boolean }> {
    return post(`/candidates/${id}/checkin`)
  },

  update(id: number, body: { name: string; profile?: string }): Promise<{ ok: boolean }> {
    return put(`/candidates/${id}`, body)
  },

  remove(id: number): Promise<{ ok: boolean }> {
    return del(`/candidates/${id}`)
  },

  resetStatus(id: number, status: CandidateStatus): Promise<{ ok: boolean }> {
    return put(`/candidates/${id}/status`, { status })
  },

  /** 候选人面试记录归档（按候选人维度，完成后仍可查；需 rooms.view）。 */
  messages(id: number): Promise<{ items: Message[] }> {
    return request(`/candidates/${id}/messages`)
  },
}

// ---- 房间 ----

export const roomApi = {
  list(params?: { limit?: number; offset?: number }): Promise<{ items: Room[] }> {
    return request({ url: '/rooms', params })
  },

  create(): Promise<{ id: number }> {
    return post('/rooms')
  },

  remove(id: number): Promise<{ ok: boolean }> {
    return del(`/rooms/${id}`)
  },

  pullCandidate(roomId: number, candidateId: number): Promise<{ ok: boolean }> {
    return post(`/rooms/${roomId}/pull_candidate`, { candidate_id: candidateId })
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
    return post(`/users/${id}/reset_password`, { new_password: newPassword })
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

export const departmentApi = {
  list(): Promise<{ items: Department[] }> {
    return request('/departments')
  },
  create(body: { name: string; description?: string }): Promise<{ id: number }> {
    return post('/departments', body)
  },
  update(id: number, body: { name: string; description?: string }): Promise<{ ok: boolean }> {
    return put(`/departments/${id}`, body)
  },
  remove(id: number): Promise<{ ok: boolean }> {
    return del(`/departments/${id}`)
  },
}
