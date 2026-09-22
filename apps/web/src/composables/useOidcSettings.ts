/**
 * useOidcSettings —— 登录认证（OIDC）设置的组合式函数（函数式 ViewModel）。
 * 资源：配置（GET /oidc/config）+ 角色名单（GET /roles）+ 部门名单（GET /departments）
 * ——后两者是两类规则的目标下拉；
 * 写入：整体保存（PUT /oidc/config）、连通性探测（POST /oidc/probes）、清除已存密钥。
 */
import { computed, ref, type Ref } from 'vue'
import { departmentApi, oidcApi, roleApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { Department, OidcConfig, OidcConfigPayload, OidcProbeResult, Role } from '@/models'

export interface UseOidcSettings {
  readonly config: Ref<OidcConfig | null>
  readonly roles: Ref<readonly Role[]>
  readonly departments: Ref<readonly Department[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  readonly saving: Ref<boolean>
  readonly probing: Ref<boolean>
  readonly probeResult: Ref<OidcProbeResult | null>
  load: () => Promise<void>
  /** 保存成功后重新拉取（密钥是否已配置以服务端为准）。 */
  save: (payload: OidcConfigPayload) => Promise<void>
  probe: (issuer: string) => Promise<void>
  /** 清除已保存的客户端密钥（省略字段=保持，显式空串=清除）。 */
  clearSecret: () => Promise<void>
}

export function useOidcSettings(): UseOidcSettings {
  const configAsync = useAsync(() => oidcApi.config())
  const roleAsync = useAsync(() => roleApi.list())
  const departmentAsync = useAsync(() => departmentApi.list())

  const saving = ref(false)
  const probing = ref(false)
  const probeResult = ref<OidcProbeResult | null>(null)

  const config = computed<OidcConfig | null>(() => configAsync.data.value)
  const roles = computed<readonly Role[]>(() => roleAsync.data.value?.items ?? [])
  const departments = computed<readonly Department[]>(() => departmentAsync.data.value?.items ?? [])
  const loading = computed(
    () => configAsync.loading.value || roleAsync.loading.value || departmentAsync.loading.value,
  )
  const error = computed(() => configAsync.error.value ?? roleAsync.error.value ?? departmentAsync.error.value)

  async function load(): Promise<void> {
    await Promise.all([configAsync.run(), roleAsync.run(), departmentAsync.run()])
  }

  async function save(payload: OidcConfigPayload): Promise<void> {
    saving.value = true
    try {
      await oidcApi.updateConfig(payload)
      await load()
    } finally {
      saving.value = false
    }
  }

  async function probe(issuer: string): Promise<void> {
    probing.value = true
    probeResult.value = null
    try {
      probeResult.value = await oidcApi.probe(issuer)
    } finally {
      probing.value = false
    }
  }

  async function clearSecret(): Promise<void> {
    const current = config.value
    if (!current) return
    await save({
      enabled: current.enabled,
      issuer: current.issuer,
      client_id: current.client_id,
      client_secret: '',
      scopes: current.scopes,
      redirect_url: current.redirect_url,
      auto_provision: current.auto_provision,
      role_rules: current.role_rules.map((r) => ({ expression: r.expression, role_id: r.role_id })),
      department_rules: current.department_rules.map((r) => ({
        expression: r.expression,
        department_id: r.department_id,
      })),
    })
  }

  return {
    config,
    roles,
    departments,
    loading,
    error,
    saving,
    probing,
    probeResult,
    load,
    save,
    probe,
    clearSecret,
  }
}
