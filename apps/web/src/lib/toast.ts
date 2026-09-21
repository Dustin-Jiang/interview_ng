/**
 * toast 提示助手（View 消费）。
 * 收敛各视图重复的 `toast.error((e as Error).message)`：异常 → 一行错误提示，
 * 非 Error 或空消息时回退到自明文案，避免出现空 toast。
 */
import { toast } from 'vue-sonner'

export function toastError(e: unknown, fallback = '操作失败'): void {
  if (e instanceof Error && e.message) {
    toast.error(e.message)
    return
  }
  if (typeof e === 'string' && e) {
    toast.error(e)
    return
  }
  toast.error(fallback)
}
