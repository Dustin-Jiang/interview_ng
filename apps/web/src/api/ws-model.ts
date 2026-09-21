/**
 * WebSocket 协议模型 —— 与后端 WS 信封契约一一对应（MVVM 的 Model 层）。
 *
 * 服务端统一推 `{ type, req_id?, data }`，其中：
 *  - type 为事件名（message_appended / room_phase_changed / ...）或 "reply"（命令回执）。
 *  - data 为对应载荷。
 *
 * 连接：`/ws/rooms/:roomId`（RESTful 路径，不带任何 query 参数）；
 * 连接建立后首条消息必须为 `auth`（携带 JWT），鉴权成功后才能收发业务命令。
 */
import type { Message, Room } from '@/models'

/** 客户端 → 服务端命令信封。 */
export interface WsCommand {
  op: 'auth' | 'sync' | 'send_msg' | 'move_phase'
  req_id?: string
  data: Record<string, unknown>
}

/** 服务端推送的事件载荷（对应后端 chanEvent）。 */
export interface ChanEvent {
  type: string
  room_id: number
  seq: number
  msg_id?: number
  data?: unknown
}

/** 命令回执载荷。 */
export interface ReplyPayload {
  ok: boolean
  error?: string
  op?: string
  room_id?: number
  room?: Room
  messages?: Message[]
  phase?: string
}
