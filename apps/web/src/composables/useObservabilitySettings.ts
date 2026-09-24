/**
 * useObservabilitySettings —— 可观测性（OTLP → GreptimeDB）设置的组合式函数（函数式 ViewModel）。
 * 资源：GET /observability/config；写入：PUT /observability/config（保存即生效，
 * 服务端重配 OTLP 推送；密码只写不读，是否已配置以服务端回发的 password_set 为准）。
 */
import { computed, ref, type Ref } from 'vue'
import { observabilityApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { ObservabilityConfig, ObservabilityConfigPayload } from '@/models'

export interface UseObservabilitySettings {
  readonly config: Ref<ObservabilityConfig | null>
  readonly loading: Ref<boolean>
  readonly saving: Ref<boolean>
  readonly applied: Ref<boolean | null> // 最近一次保存的即时生效结果（null = 尚未保存过）
  load: () => Promise<void>
  /** 保存成功后重新拉取（密码是否已配置以服务端为准），并回发 applied 结果。 */
  save: (payload: ObservabilityConfigPayload) => Promise<boolean>
}

export function useObservabilitySettings(): UseObservabilitySettings {
  const configAsync = useAsync(() => observabilityApi.config())
  const saving = ref(false)
  const applied = ref<boolean | null>(null)

  const config = computed<ObservabilityConfig | null>(() => configAsync.data.value)
  const loading = computed(() => configAsync.loading.value)

  async function load(): Promise<void> {
    await configAsync.run()
  }

  async function save(payload: ObservabilityConfigPayload): Promise<boolean> {
    saving.value = true
    try {
      const res = await observabilityApi.updateConfig(payload)
      await configAsync.run()
      applied.value = res.applied
      return res.applied
    } finally {
      saving.value = false
    }
  }

  return { config, loading, saving, applied, load, save }
}
