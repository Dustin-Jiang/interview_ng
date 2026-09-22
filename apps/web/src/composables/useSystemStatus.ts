/**
 * useSystemStatus —— 系统状态（阶段 / 出价步长）的**唯一共享数据源**。
 *
 * 系统状态是单行资源，多页都要读：
 *  - 系统设置页：展示并切换阶段、改出价步长（唯一写入方）；
 *  - 捡漏页：判定是否处于捡漏阶段（能否出价）+ 出价步长；
 *  - 候选人页：录取阶段是否展示录取控件。
 *
 * 因此状态放在**模块级单例**（与 `useBoardChannel` 同款）：谁先访问谁触发一次拉取，
 * 之后所有消费方共享同一份 `status` 与同一次刷新（并发的 `load()` 自动合并），
 * 不再各页各拉一份、各留一个影子副本。
 * 写入方（`setPhase`）在落库后强制刷新，消费方无需额外同步。
 *
 * 注意：没有「阶段变更」的 WS 事件，阶段由管理员在设置页切换，
 * 因此需要新鲜度的页面（如捡漏页的整页重拉）应把 `load()` 一并纳入自己的刷新流程。
 */
import { computed, type ComputedRef, type Ref } from 'vue'
import { systemStatusApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { SystemPhase, SystemStatus } from '@/models'

/** 出价步长缺省值（后端默认 10；非法/缺失时回退）。 */
const DEFAULT_BID_STEP = 10

const status = useAsync(() => systemStatusApi.get())
let inflight: Promise<SystemStatus | null> | null = null

const current = computed<SystemStatus | null>(() => status.data.value)

/** 拉取（并发去重）：多处同时调用只发一次请求。 */
async function load(): Promise<SystemStatus | null> {
  if (inflight) return inflight
  inflight = status.run().finally(() => {
    inflight = null
  })
  return inflight
}

export interface UseSystemStatus {
  readonly status: Ref<SystemStatus | null>
  /** 当前系统阶段（未加载为 null）。 */
  readonly phase: ComputedRef<SystemPhase | null>
  /** 出价步长（≥1，缺省 10）。 */
  readonly bidStep: ComputedRef<number>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  load: () => Promise<SystemStatus | null>
  setPhase: (phase: SystemPhase) => Promise<void>
}

export function useSystemStatus(): UseSystemStatus {
  // 首次访问即拉取一次（模块级状态，全局只此一份）。
  if (!current.value && !inflight) void load()

  const phase = computed<SystemPhase | null>(() => current.value?.phase ?? null)
  const bidStep = computed<number>(() => {
    const step = current.value?.bid_step
    return step && step > 0 ? step : DEFAULT_BID_STEP
  })

  /** 切换阶段：落库后强制刷新（绕过并发去重，避免拿到切换前的 in-flight 结果）。 */
  async function setPhase(next: SystemPhase): Promise<void> {
    await systemStatusApi.patch({ phase: next })
    await status.run()
  }

  return {
    status: current,
    phase,
    bidStep,
    loading: status.loading,
    error: status.error,
    load,
    setPhase,
  }
}
