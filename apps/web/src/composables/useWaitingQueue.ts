/**
 * useWaitingQueue —— 候场队列（「已签到待分配」档）的唯一共享数据源。
 *
 * 模块级单例 + 引用计数（同 `useSystemStatus` / `useBoardChannel` 手法）：候场大屏与房间侧栏
 * 看到的是**同一份、同一顺序**的队列，且只有一条刷新链路——看板通道事件防抖重拉
 * （`useBoardChannel` 是模块级单例，本组合式按消费者引用计数复用同一条连接）。
 *
 * 为什么不再各自拉一份：拉取池曾在 `useRoomChat` 里自带 fetch + 自带事件订阅，
 * 与候场大屏成了两条独立链路——同一次调序，两边可能一个已刷新一个没刷新（顺序不一致），
 * 且并发重拉时旧响应会覆盖新顺序。收敛到本组合式后不存在第二个写者。
 *
 * 顺序口径 = `domain/status.ts#compareWaiting`（与后端 `mem_store.go#compareWaiting` 逐项同构）；
 * 名次即数组下标 + 1（`RoomSidebar` 直接按下标显示 1..N）。
 */
import { onScopeDispose, ref, type Ref } from 'vue'

import { candidateApi } from '@/api/http'
import type { Candidate } from '@/models'
import { QUEUE_BOARD_EVENTS, sortWaitingPool } from '@/domain/status'
import { useBoardChannel } from '@/composables/useBoardChannel'

export interface UseWaitingQueue {
  /** 候场队列（「已签到待分配」档，按队列顺序；下标 + 1 = 名次）。 */
  readonly queue: Ref<Candidate[]>
  /** 首次/重拉中（侧栏据此决定骨架或沿用旧数据）。 */
  readonly loading: Ref<boolean>
  /** 立即重拉（自己写完队列相关操作后调用，不等事件；其余端由事件刷新）。 */
  reload: () => Promise<void>
}

const queue = ref<Candidate[]>([])
const loading = ref(false)

/** 消费者数量：>0 时保持事件订阅，归零时注销（数据保留，重挂不闪空态）。 */
let refCount = 0
let timer: ReturnType<typeof setTimeout> | null = null
/** 请求序号：并发重拉时只接受最后发起的那次响应（旧响应会把新顺序覆盖回去）。 */
let seq = 0

async function load(): Promise<void> {
  const mine = ++seq
  loading.value = true
  try {
    const res = await candidateApi.listAll({ status: 'CHECKED_IN_PENDING_ASSIGN' })
    if (mine !== seq) return
    queue.value = sortWaitingPool(res.items)
  } finally {
    if (mine === seq) loading.value = false
  }
}

/** 事件风暴合并（300ms，与 `useBoardRefresh` 同一节奏）。 */
function scheduleReload(): void {
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => {
    timer = null
    void load()
  }, 300)
}

export function useWaitingQueue(): UseWaitingQueue {
  // 每个消费者各持一份订阅与引用计数（useBoardChannel 内部按消费者增减，连接仍是单例）。
  const off = useBoardChannel().subscribe((ev) => {
    if (QUEUE_BOARD_EVENTS.includes(ev.type)) scheduleReload()
  })
  refCount++
  onScopeDispose(() => {
    off()
    refCount--
    if (refCount === 0 && timer) {
      clearTimeout(timer)
      timer = null
    }
  })
  // 首个消费者挂载即拉一次：模块级缓存可能是上次会话的旧数据。
  if (refCount === 1) void load()

  return { queue, loading, reload: load }
}
