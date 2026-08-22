/**
 * WebSocket 通道服务 —— MVVM 的 Service（Model 访问）层。
 * 职责：管理一条到后端的 WS 长连接，收发信封、断线重连。
 * 连接建立后先发 auth 消息（JWT），鉴权成功后才进入业务阶段；
 * 鉴权前发出的命令（如 useRoomChat 在 connect() 后立即 sync(0)）暂存，
 * 待 auth 回执成功后再按序 flush —— 否则会被 socket 未 OPEN 的 rawSend 静默丢弃。
 * 与框架解耦：不发 Vue 响应式状态，只通过回调把原始事件/回执上抛。
 */
import type { ChanEvent, ReplyPayload, WsCommand } from './ws-model'

export interface WsHandlers {
  /** 收到一条服务端推送的频道事件（非 reply）。 */
  onEvent?: (event: ChanEvent) => void
  /** 收到一条命令回执。 */
  onReply?: (reqId: string, payload: ReplyPayload) => void
  /** 连接建立（含重连成功）。 */
  onOpen?: () => void
  /** 连接关闭（含断线）。 */
  onClose?: () => void
  /** 连接层致命错误（如鉴权被拒）。 */
  onFatal?: (message: string) => void
  onError?: (err: Event) => void
}

export class RoomChannel {
  private ws: WebSocket | null = null
  private closed = false
  private seqCounter = 0
  /** 鉴权成功前发出的命令暂存于此，auth 回执成功后按序发送。 */
  private pending: WsCommand[] = []
  private authReqId = ''
  private authed = false

  constructor(
    private roomId: number,
    private token: string,
    private handlers: WsHandlers = {},
  ) {}

  connect(): void {
    this.closed = false
    this.authed = false
    this.pending = []
    // RESTful 路径承载 roomId；token 走连接后 auth 消息，不进 URL。
    const ws = new WebSocket(`/ws/room/${this.roomId}`)
    this.ws = ws

    ws.onopen = () => {
      // 首条消息必须是 auth。
      this.authReqId = this.nextReqId()
      this.rawSend({ op: 'auth', req_id: this.authReqId, data: { token: this.token } })
      this.handlers.onOpen?.()
    }
    ws.onmessage = (e) => this.handleMessage(String(e.data))
    ws.onclose = () => {
      this.handlers.onClose?.()
      // 非主动关闭 → 定时重连
      if (!this.closed) setTimeout(() => this.connect(), 1500)
    }
    ws.onerror = (e) => this.handlers.onError?.(e)
  }

  private nextReqId(): string {
    return `r${Date.now()}_${++this.seqCounter}`
  }

  private rawSend(frame: WsCommand): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
    this.ws.send(JSON.stringify(frame))
  }

  /** auth 成功回执到达后，flush 暂存命令。 */
  private flush(): void {
    const buf = this.pending
    this.pending = []
    for (const frame of buf) {
      this.rawSend(frame)
    }
  }

  private handleMessage(raw: string): void {
    let env: { type: string; req_id?: string; data?: unknown }
    try {
      env = JSON.parse(raw)
    } catch {
      return
    }
    if (env.type === 'reply') {
      const payload = (env.data as ReplyPayload) ?? { ok: false }
      // 鉴权成功 → 解除命令闸门并 flush 暂存命令。
      if (env.req_id === this.authReqId && payload.ok) {
        this.authed = true
        this.flush()
      }
      this.handlers.onReply?.(env.req_id ?? '', payload)
      return
    }
    // 后端在鉴权/入房失败时下发 {"type":"sync","data":{"error":...}}
    const errMsg = (env.data as { error?: string } | undefined)?.error
    if (errMsg) {
      this.handlers.onFatal?.(errMsg)
      return
    }
    this.handlers.onEvent?.(env.data as ChanEvent)
  }

  /** 发送命令；返回自动生成的 req_id。鉴权完成前暂存。 */
  send(op: WsCommand['op'], data: Record<string, unknown>): string {
    const reqId = this.nextReqId()
    const frame: WsCommand = { op, req_id: reqId, data }
    if (this.authed && this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.rawSend(frame)
    } else {
      this.pending.push(frame)
    }
    return reqId
  }

  sync(lastMsgId: number): string {
    return this.send('sync', { room_id: this.roomId, last_msg_id: lastMsgId, last_seq: 0 })
  }

  sendMessage(content: string): string {
    return this.send('send_msg', { room_id: this.roomId, content })
  }

  movePhase(to: string): string {
    return this.send('move_phase', { room_id: this.roomId, to })
  }

  close(): void {
    this.closed = true
    this.ws?.close()
    this.ws = null
  }
}
