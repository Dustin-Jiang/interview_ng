/**
 * useUrlSync —— 把视图状态防抖写回 URL（replace，不污染历史）。
 * 供「筛选 / 选中条目」等可深链状态使用：来源变化 300ms 后整体替换 location。
 */
import { onScopeDispose, watch } from 'vue'
import { useRouter, type RouteLocationRaw } from 'vue-router'

import { useDebouncedRefresh } from '@/composables/useDebouncedRefresh'

/**
 * @param build 生成完整目标 location（路径参数 + query，空值省略）。
 * @param sources 变化来源 getter（返回依赖值，任一变化即调度一次写回）。
 */
export function useUrlSync(build: () => RouteLocationRaw, sources: () => unknown): void {
  const router = useRouter()
  const debounced = useDebouncedRefresh(() => {
    void router.replace(build())
  })

  const stop = watch(sources, () => debounced.schedule())
  onScopeDispose(stop)
}
