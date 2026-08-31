/**
 * useSystemStatus —— 系统状态（面试阶段 / 录取阶段 / 捡漏阶段）组合式函数。
 * 读取状态并在各档之间切换；切换后重新拉取刷新当前阶段。
 */
import { computed, type Ref } from 'vue'
import { systemStatusApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { SystemPhase, SystemStatus } from '@/models'

export interface UseSystemStatus {
  readonly status: Ref<SystemStatus | null>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  load: () => Promise<SystemStatus | null>
  setPhase: (phase: SystemPhase) => Promise<void>
}

export function useSystemStatus(): UseSystemStatus {
  const status = useAsync(() => systemStatusApi.get())

  const current = computed<SystemStatus | null>(() => status.data.value)

  async function load(): Promise<SystemStatus | null> {
    return status.run()
  }

  async function setPhase(phase: SystemPhase): Promise<void> {
    await systemStatusApi.set(phase)
    await load()
  }

  return {
    status: current,
    loading: status.loading,
    error: status.error,
    load,
    setPhase,
  }
}