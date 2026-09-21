/**
 * useConfirmAction —— 二次确认动作（ConfirmDialog）状态组合式函数。
 * 收编各视图重复的 `target / loading / confirm` 三件套：目标由 request() 置入，
 * 确认后执行 action 并清空，关闭对话框即取消。删除、结算等破坏性/不可逆动作共用。
 */
import { ref, type Ref } from 'vue'
import { toast } from 'vue-sonner'

import { toastError } from '@/lib/toast'

export interface UseConfirmAction<T> {
  /** 待处理目标（null = 对话框关闭）。 */
  readonly target: Ref<T | null>
  readonly loading: Ref<boolean>
  /** 打开确认对话框。 */
  request: (target: T) => void
  /** ConfirmDialog 的 update:open 回调（关闭即清空目标）。 */
  onOpenChange: (open: boolean) => void
  /** 执行动作（失败 toast 错误）。 */
  confirm: () => Promise<void>
}

export function useConfirmAction<T>(opts: {
  /** 确认后执行的动作；成功与否由它自行决定（含 reload）。 */
  action: (target: T) => Promise<void>
  /** 成功提示文案；省略则仅处理异常（动作内部已给出更具体的成功提示）。 */
  success?: (target: T) => string
}): UseConfirmAction<T> {
  const target = ref<T | null>(null) as Ref<T | null>
  const loading = ref(false)

  function request(value: T): void {
    target.value = value
  }

  function onOpenChange(open: boolean): void {
    if (!open && !loading.value) target.value = null
  }

  async function confirm(): Promise<void> {
    const value = target.value
    if (!value || loading.value) return
    loading.value = true
    try {
      await opts.action(value)
      if (opts.success) toast.success(opts.success(value))
      target.value = null
    } catch (e) {
      toastError(e)
    } finally {
      loading.value = false
    }
  }

  return { target, loading, request, onOpenChange, confirm }
}
