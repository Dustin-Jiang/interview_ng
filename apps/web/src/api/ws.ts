/**
 * WebSocket 通道服务 —— MVVM 的 Service（Model 访问）层。
 * 职责：管理一条到后端的 WS 长连接，收发信封、断线重连。
 * 与框架解耦：不发 Vue 响应式状态，只通过回调把原始事件/回执上抛，
 * 由 ViewModel（store）汇聚成可绑定的状态。
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
  /** 连接层致命错误（如加入房间被拒），进入后注入但非事件/回执的信封。 */
  onFatal?: (message: string) => void
  onError?: (err: Event) => void
}

export class RoomChannel {
  private ws: WebSocket | null = null
  private closed = false
  private seqCounter = 0
  /** 在连接建立前发出的命令暂存于此，open 后按序发送。 */
  private pending: WsCommand[] = []

  constructor(
    private url: string,
    private handlers: WsHandlers = {},
  ) {}

  connect(): void {
    this.closed = false
    const ws = new WebSocket(this.url)
    this.ws = ws

    ws.onopen = () => {
      this.flush()
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

  private flush(): void {
    const buf = this.pending
    this.pending = []
    for (const cmd of buf) {
      this.rawSend(cmd)
    }
  }

  private rawSend(frame: WsCommand): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) return
    this.ws.send(JSON.stringify(frame))
  }

  private handleMessage(raw: string): void {
    let env: { type: string; req_id?: string; data?: unknown }
    try {
      env = JSON.parse(raw)
    } catch {
      return
    }
    if (env.type === 'reply') {
      this.handlers.onReply?.(env.req_id ?? '', (env.data as ReplyPayload) ?? { ok: false })
      return
    }
    // 后端在升级失败/加入被拒时会下发 {"type":"sync","data":{"error":...}}
    const errMsg = (env.data as { error?: string } | undefined)?.error
    if (errMsg) {
      this.handlers.onFatal?.(errMsg)
      return
    }
    this.handlers.onEvent?.(env.data as ChanEvent)
  }

  /** 发送命令；返回自动生成的 req_id。连接未就绪时先入队，open 后发送。 */
  send(op: WsCommand['op'], data: Record<string, unknown>): string {
    const reqId = `r${Date.now()}_${++this.seqCounter}`
    const frame: WsCommand = { op, req_id: reqId, data }
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.rawSend(frame)
    } else {
      this.pending.push(frame)
    }
    return reqId
  }

  sync(lastMsgId: number, roomId: number): string {
    return this.send('sync', { room_id: roomId, last_msg_id: lastMsgId, last_seq: 0 })
  }

  sendMessage(roomId: number, content: string): string {
    return this.send('send_msg', { room_id: roomId, content })
  }

  join(roomId: number): string {
    return this.send('join', { room_id: roomId })
  }

  movePhase(roomId: number, to: string): string {
    return this.send('move_phase', { room_id: roomId, to })
  }

  close(): void {
    this.closed = true
    this.ws?.close()
    this.ws = null
  }
}
