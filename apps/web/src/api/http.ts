/**
 * HTTP API 客户端 —— MVVM 的 Service（Model 访问）层。
 * 职责：封装对后端 REST 接口的访问，只做数据收发与类型映射，不含任何 UI 状态。
 * 鉴权：自动携带 Authorization: Bearer token；401 时回调统一登出（由 useAuth 注册）。
 */
import type { Candidate, CandidateStatus, Permission, Role, Room, User, UserProfile } from '@/models'

const BASE = '/api'

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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (currentToken) headers.Authorization = `Bearer ${currentToken}`
  const res = await fetch(`${BASE}${path}`, { headers, ...init })
  if (res.status === 401) {
    onUnauthorized?.()
  }
  if (!res.ok) {
    let msg = res.statusText
    try {
      const body = await res.json()
      msg = body?.error ?? msg
    } catch {
      /* ignore */
    }
    throw new Error(msg || `HTTP ${res.status}`)
  }
  return res.json() as Promise<T>
}

function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, { method: 'POST', body: body == null ? undefined : JSON.stringify(body) })
}

function put<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, { method: 'PUT', body: body == null ? undefined : JSON.stringify(body) })
}

function del<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'DELETE' })
}

// ---- 认证 ----

export const authApi = {
  login(username: string, password: string): Promise<UserProfile & { token: string }> {
    return post('/auth/login', { username, password })
  },
  me(): Promise<UserProfile> {
    return request('/me')
  },
  changePassword(oldPassword: string, newPassword: string): Promise<{ ok: boolean }> {
    return post('/auth/password', { old_password: oldPassword, new_password: newPassword })
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
    const q = new URLSearchParams()
    if (params?.status) q.set('status', params.status)
    if (params?.q) q.set('q', params.q)
    if (params?.limit != null) q.set('limit', String(params.limit))
    if (params?.offset != null) q.set('offset', String(params.offset))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request(`/candidates${suffix}`)
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
}

// ---- 房间 ----

export const roomApi = {
  list(params?: { limit?: number; offset?: number }): Promise<{ items: Room[] }> {
    const q = new URLSearchParams()
    if (params?.limit != null) q.set('limit', String(params.limit))
    if (params?.offset != null) q.set('offset', String(params.offset))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request(`/rooms${suffix}`)
  },

  get(id: number): Promise<Room> {
    return request(`/rooms/${id}`)
  },

  create(): Promise<{ id: number }> {
    return post('/rooms')
  },

  remove(id: number): Promise<{ ok: boolean }> {
    return del(`/rooms/${id}`)
  },

  addMember(roomId: number, userId: number): Promise<{ ok: boolean }> {
    return post(`/rooms/${roomId}/members`, { user_id: userId })
  },

  removeMember(roomId: number, userId: number): Promise<{ ok: boolean }> {
    return del(`/rooms/${roomId}/members/${userId}`)
  },

  setCurrentInterviewer(roomId: number, interviewerId: number): Promise<{ ok: boolean }> {
    return put(`/rooms/${roomId}/current_interviewer`, { interviewer_id: interviewerId })
  },

  pullCandidate(roomId: number, candidateId: number): Promise<{ ok: boolean }> {
    return post(`/rooms/${roomId}/pull_candidate`, { candidate_id: candidateId })
  },
}

// ---- 面试官与角色管理 ----

export const userApi = {
  list(params?: { q?: string; limit?: number; offset?: number }): Promise<{ items: User[] }> {
    const q = new URLSearchParams()
    if (params?.q) q.set('q', params.q)
    if (params?.limit != null) q.set('limit', String(params.limit))
    if (params?.offset != null) q.set('offset', String(params.offset))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request(`/users${suffix}`)
  },
  create(body: { username: string; name: string; password: string; role_ids: number[] }): Promise<{ id: number }> {
    return post('/users', body)
  },
  update(id: number, body: { name: string; role_ids: number[] }): Promise<{ ok: boolean }> {
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
