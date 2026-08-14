/**
 * HTTP API 客户端 —— MVVM 的 Service（Model 访问）层。
 * 职责：封装对后端 REST 接口的访问，只做数据收发与类型映射，不含任何 UI 状态。
 */
import type { Candidate, CandidateStatus } from '@/models'

const BASE = '/api'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
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

export const candidateApi = {
  list(params?: { status?: CandidateStatus; limit?: number; offset?: number }): Promise<{ items: Candidate[] }> {
    const q = new URLSearchParams()
    if (params?.status) q.set('status', params.status)
    if (params?.limit != null) q.set('limit', String(params.limit))
    if (params?.offset != null) q.set('offset', String(params.offset))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request(`/candidates${suffix}`)
  },

  get(id: number): Promise<Candidate> {
    return request(`/candidates/${id}`)
  },

  create(body: { name: string; profile?: string }): Promise<{ id: number }> {
    return request('/candidates', { method: 'POST', body: JSON.stringify(body) })
  },

  checkin(id: number): Promise<{ ok: boolean }> {
    return request(`/candidates/${id}/checkin`, { method: 'POST' })
  },

  assign(id: number, roomId?: number): Promise<{ ok: boolean; room_id: number }> {
    return request(`/candidates/${id}/assign`, {
      method: 'POST',
      body: JSON.stringify({ room_id: roomId ?? null }),
    })
  },
}

export const roomApi = {
  list(params?: { limit?: number; offset?: number }): Promise<{ items: import('@/models').Room[] }> {
    const q = new URLSearchParams()
    if (params?.limit != null) q.set('limit', String(params.limit))
    if (params?.offset != null) q.set('offset', String(params.offset))
    const suffix = q.toString() ? `?${q.toString()}` : ''
    return request(`/rooms${suffix}`)
  },

  get(id: number): Promise<import('@/models').Room> {
    return request(`/rooms/${id}`)
  },
}
