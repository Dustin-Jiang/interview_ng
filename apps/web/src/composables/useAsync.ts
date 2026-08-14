/**
 * useAsync —— 通用异步状态组合式函数（函数式）。
 * 把"加载中 / 错误 / 数据"这套常见状态封装成可组合原语，供各业务 useXXX() 复用。
 *
 * 函数式要点：
 *  - loader 是纯函数：给定阶段返回 Promise<数据>；
 *  - 返回的方法都返回 Promise，支持链式组合与 await（view 可 await store 化透传）。
 *  - 状态均为只读 ref 暴露，数据更新走 run()，避免外部命令式篡改。
 */
import { ref, type Ref } from 'vue'

export interface AsyncState<T> {
  /** 数据（未加载为 null）。列表场景下 loader 返回数组，data.value 即数组。 */
  data: Ref<T | null>
  /** 是否正在加载。 */
  loading: Ref<boolean>
  /** 错误信息（空则为无错）。 */
  error: Ref<string | null>
  /** 执行加载并写入 data。返回数据或 promise 拒绝信息。 */
  run: () => Promise<T | null>
  /** 手动清空数据与错误。 */
  reset: () => void
}

function asMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e)
}

/**
 * 封装一个异步资源。
 * @param loader 无参纯函数：返回 Promise<数据>。闭包捕获外部可观察状态（如筛选条件）。
 */
export function useAsync<T>(loader: () => Promise<T>): AsyncState<T> {
  const data = ref<T | null>(null) as Ref<T | null>
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function run(): Promise<T | null> {
    loading.value = true
    error.value = null
    try {
      const result = await loader()
      data.value = result
      return result
    } catch (e) {
      error.value = asMessage(e)
      return null
    } finally {
      loading.value = false
    }
  }

  function reset(): void {
    data.value = null
    error.value = null
    loading.value = false
  }

  return { data, loading, error, run, reset }
}
