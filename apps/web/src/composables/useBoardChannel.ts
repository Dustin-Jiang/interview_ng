/**
 * useBoardChannel —— 看板 WS 通道组合式函数（模块级单例 + 引用计数）。
 * 一条到 /ws/board 的长连接向所有业务列表页扇出全局事件
 * （签到 / 拉取 / 阶段变化 / 候选人与房间 CRUD），页面据此防抖重拉列表。
 *
 * - subscribe 注册事件回调，返回取消函数；作用域销毁自动清理并递减引用；
 * - 引用归零时断开连接；下次使用以当前 token 重建（覆盖重新登录场景）；
 * - 无 rooms.view 权限等致命错误：通道静默停连，界面退化为手动刷新。
 */
import { onScopeDispose, ref, type Ref } from 'vue'
import { getAuthToken } from '@/api/http'
import { WsChannel } from '@/api/ws'
import type { ChanEvent } from '@/api/ws-model'

export type BoardHandler = (ev: ChanEvent) => void

export interface UseBoardChannel {
  /** 通道连接状态（未授权时恒为 false，不影响手动刷新）。 */
  readonly connected: Ref<boolean>
  /** 订阅看板事件，返回取消函数。 */
  subscribe: (handler: BoardHandler) => () => void
}

let channel: WsChannel | null = null
let refCount = 0
const handlers = new Map<symbol, BoardHandler>()
const connected = ref(false)

function ensureChannel(): void {
  if (channel) return
  const ch = new WsChannel('/ws/board', getAuthToken(), {
    onOpen: () => {
      connected.value = true
    },
    onClose: () => {
      connected.value = false
    },
    onEvent: (ev) => {
      for (const h of handlers.values()) h(ev)
    },
    // 致命错误（无权限/鉴权失败）：WsChannel 已停止重连，此处静默降级。
    onFatal: () => {
      connected.value = false
    },
  })
  ch.connect()
  channel = ch
}

function releaseChannel(): void {
  if (refCount > 0) return
  channel?.close()
  channel = null
  connected.value = false
}

export function useBoardChannel(): UseBoardChannel {
  refCount++
  ensureChannel()

  const unsubscribeList: Array<() => void> = []
  function subscribe(handler: BoardHandler): () => void {
    const key = Symbol()
    handlers.set(key, handler)
    const cancel = () => handlers.delete(key)
    unsubscribeList.push(cancel)
    return cancel
  }

  onScopeDispose(() => {
    for (const c of unsubscribeList) c()
    refCount--
    releaseChannel()
  })

  return { connected, subscribe }
}

/**
 * useBoardRefresh —— 订阅指定类型的看板事件并防抖触发回调（300ms 合并风暴）。
 * 供列表页"事件到达 → 整表重拉"的标准模式使用；作用域销毁自动清理定时器与订阅。
 */
export function useBoardRefresh(events: readonly string[], cb: () => void): UseBoardChannel {
  let timer: ReturnType<typeof setTimeout> | null = null
  const schedule = (): void => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(() => {
      timer = null
      cb()
    }, 300)
  }
  const board = useBoardChannel()
  board.subscribe((ev) => {
    if (events.includes(ev.type)) schedule()
  })
  onScopeDispose(() => {
    if (timer) clearTimeout(timer)
  })
  return board
}
