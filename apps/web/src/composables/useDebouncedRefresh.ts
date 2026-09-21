/**
 * useDebouncedRefresh —— 防抖触发回调（默认 300ms 合并风暴）。
 * 事件风暴场景（WS 事件、URL 同步）的通用原语：作用域销毁自动清理定时器。
 */
import { onScopeDispose } from 'vue'

export interface DebouncedRefresh {
  /** 触发一次（合并窗口内的重复调用只执行最后一次）。 */
  schedule: () => void
  /** 立即执行挂起的一次（若存在）。 */
  flush: () => void
  /** 取消挂起的一次。 */
  cancel: () => void
}

export function useDebouncedRefresh(cb: () => void, delay = 300): DebouncedRefresh {
  let timer: ReturnType<typeof setTimeout> | null = null

  function cancel(): void {
    if (timer) {
      clearTimeout(timer)
      timer = null
    }
  }

  function schedule(): void {
    cancel()
    timer = setTimeout(() => {
      timer = null
      cb()
    }, delay)
  }

  function flush(): void {
    if (!timer) return
    cancel()
    cb()
  }

  onScopeDispose(cancel)

  return { schedule, flush, cancel }
}
