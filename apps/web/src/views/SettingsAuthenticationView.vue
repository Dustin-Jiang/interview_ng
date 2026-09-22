<!--
  SettingsAuthenticationView —— 「登录认证」设置分区（users.manage）：
  单点登录（OIDC）连接参数（开关 / Issuer / 客户端 / Scopes / 回调地址 / 自动开通 + 连通性检测）
  + 「声明 → 角色」与「声明 → 部门」两张 JMESPath 规则表（各自自上而下首个命中生效；
  规则改动**即时落库**，连接参数仍走卡片里的「保存」）
  + 规则验证（粘贴 ID token 声明试算两类规则的命中结果）。
  客户端密钥只写不读：界面只显示「是否已配置」，不回收明文。
  回调地址只让改**主机**：路径固定为后端回调端点（OIDC_CALLBACK_PATH），管理员无从改错。
-->
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'

import { OIDC_CALLBACK_PATH } from '@/api/http'
import { useConfirmAction } from '@/composables/useConfirmAction'
import { useOidcSettings } from '@/composables/useOidcSettings'
import { callbackOriginOf, normalizeCallbackOrigin } from '@/domain/oidc'
import { toastError } from '@/lib/toast'
import type { OidcConfig, OidcConfigPayload, OidcDeptRulePayload, OidcRulePayload } from '@/models'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import ListSkeleton from '@/components/app/ListSkeleton.vue'
import OidcClaimsPreview from '@/components/app/OidcClaimsPreview.vue'
import OidcRuleTable from '@/components/app/OidcRuleTable.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'

const {
  config,
  roles,
  departments,
  loading,
  saving,
  probing,
  probeResult,
  load,
  save,
  probe,
  clearSecret,
} = useOidcSettings()

onMounted(() => void load())

/** 两类规则表的目标下拉（角色 / 部门），规则里只存 id。 */
const roleTargets = computed(() => roles.value.map((r) => ({ id: r.id, label: r.name })))
const departmentTargets = computed(() => departments.value.map((d) => ({ id: d.id, label: d.name })))

/** 回调主机默认值：控制台自身来源（后端用回调地址的主机推导登录页来源，二者必须同源）。 */
function defaultOrigin(): string {
  return window.location.origin
}

const form = ref({
  enabled: false,
  issuer: '',
  client_id: '',
  /** 草稿态密钥：空串 = 不修改（清除走单独按钮）。 */
  client_secret: '',
  scopes: 'openid profile email',
  /** 回调地址的**主机部分**（scheme://host[:port]）；路径固定为 OIDC_CALLBACK_PATH。 */
  callback_origin: '',
  auto_provision: true,
  role_rules: [] as OidcRulePayload[],
  department_rules: [] as OidcDeptRulePayload[],
})

// 服务端配置为准：加载/保存后重填草稿（主机取自已存回调地址，路径不参与编辑）。
watch(
  config,
  (c) => {
    if (!c) return
    form.value = {
      enabled: c.enabled,
      issuer: c.issuer,
      client_id: c.client_id,
      client_secret: '',
      scopes: c.scopes || 'openid profile email',
      callback_origin: callbackOriginOf(c.redirect_url) || defaultOrigin(),
      auto_provision: c.auto_provision,
      role_rules: c.role_rules.map((r) => ({ expression: r.expression, role_id: r.role_id })),
      department_rules: c.department_rules.map((r) => ({
        expression: r.expression,
        department_id: r.department_id,
      })),
    }
  },
  { immediate: true },
)

const secretPlaceholder = computed(() =>
  config.value?.client_secret_set ? '已配置（留空则不修改）' : '客户端密钥（公开客户端可留空）',
)

/** 由「已保存的配置」组装 PUT 体（PUT 是整体覆盖，缺省值即服务端现值；客户端密钥省略 = 保持）。 */
function payloadOf(cfg: OidcConfig, overrides: Partial<OidcConfigPayload> = {}): OidcConfigPayload {
  return {
    enabled: cfg.enabled,
    issuer: cfg.issuer,
    client_id: cfg.client_id,
    scopes: cfg.scopes,
    redirect_url: cfg.redirect_url,
    auto_provision: cfg.auto_provision,
    role_rules: cfg.role_rules.map((r) => ({ expression: r.expression, role_id: r.role_id })),
    department_rules: cfg.department_rules.map((r) => ({
      expression: r.expression,
      department_id: r.department_id,
    })),
    ...overrides,
  }
}

/**
 * 规则表改动**即时落库**：规则卡片没有自己的保存按钮，新增/编辑/删除/调序若只改草稿，
 * 加完规则会以为已经存下了（实际还得回上方的连接参数卡片点「保存」）。
 * 只覆盖被改动的规则数组——不顺手把连接参数里未保存的编辑一起写进去。
 */
async function persistRules(
  overrides: Partial<Pick<OidcConfigPayload, 'role_rules' | 'department_rules'>>,
): Promise<void> {
  const cfg = config.value
  if (!cfg) return
  try {
    await save(payloadOf(cfg, overrides))
    toast.success('规则已保存')
  } catch (e) {
    toastError(e)
  }
}

async function submit(): Promise<void> {
  const cfg = config.value
  const origin = normalizeCallbackOrigin(form.value.callback_origin)
  if (!cfg) return
  if (!origin) {
    toast.error('回调地址需以 http:// 或 https:// 开头，例如 https://interview.example.com')
    return
  }
  const payload = payloadOf(cfg, {
    enabled: form.value.enabled,
    issuer: form.value.issuer.trim(),
    client_id: form.value.client_id.trim(),
    scopes: form.value.scopes.trim(),
    // 路径固定，只换主机：拼出来的一定是后端真实回调端点。
    redirect_url: origin + OIDC_CALLBACK_PATH,
    auto_provision: form.value.auto_provision,
    role_rules: form.value.role_rules,
    department_rules: form.value.department_rules,
  })
  // 省略该字段 = 保持原密钥；清空请用「清除密钥」。
  if (form.value.client_secret) payload.client_secret = form.value.client_secret
  try {
    await save(payload)
    toast.success('已保存')
  } catch (e) {
    toastError(e)
  }
}

async function runProbe(): Promise<void> {
  try {
    await probe(form.value.issuer.trim())
    toast.success('已获取发现文档')
  } catch (e) {
    toastError(e)
  }
}

const {
  target: clearSecretTarget,
  loading: clearingSecret,
  request: requestClearSecret,
  onOpenChange: onClearSecretOpenChange,
  confirm: confirmClearSecret,
} = useConfirmAction<true>({
  action: async () => {
    await clearSecret()
  },
  success: () => '客户端密钥已清除',
})
</script>

<template>
  <PageShell title="登录认证">
    <template #actions>
      <RefreshButton label="刷新登录认证配置" :loading="loading" @click="load" />
    </template>

    <ListSkeleton v-if="loading && !config" :rows="2" item-class="h-40 w-full rounded-xl" class="space-y-4" />

    <template v-else-if="config">
      <!-- 连接参数 -->
      <Card>
        <CardHeader>
          <CardTitle class="text-base">单点登录（OIDC）</CardTitle>
        </CardHeader>
        <CardContent class="grid gap-4">
          <!-- 触屏（≤lg）整行即命中区：把 label 拉满行高，而不是只靠复选框那一小块。 -->
          <label class="flex max-lg:min-h-11 items-center gap-2">
            <input v-model="form.enabled" type="checkbox" class="size-4 accent-primary max-lg:size-5" />
            启用统一身份认证
          </label>

          <div class="grid gap-2">
            <Label for="oidc-issuer">Issuer</Label>
            <Input id="oidc-issuer" v-model="form.issuer" placeholder="https://sso.example.com/realms/interview" />
          </div>

          <div class="grid gap-2">
            <Label for="oidc-client-id">Client ID</Label>
            <Input id="oidc-client-id" v-model="form.client_id" placeholder="interview-ng" />
          </div>

          <div class="grid gap-2">
            <Label for="oidc-client-secret">Client Secret</Label>
            <Input
              id="oidc-client-secret"
              v-model="form.client_secret"
              type="password"
              autocomplete="new-password"
              :placeholder="secretPlaceholder"
            />
          </div>

          <div class="grid gap-2">
            <Label for="oidc-scopes">Scopes</Label>
            <Input id="oidc-scopes" v-model="form.scopes" placeholder="openid profile email" />
          </div>

          <div class="grid gap-2">
            <Label for="oidc-callback-origin">回调地址</Label>
            <!-- 主机可编辑，路径固定（后端回调端点，注册到 IdP 的 redirect_uri 即此完整地址）。
                 手机上窄屏放不下「输入框 + 路径后缀」这一行，故纵向堆叠：两段各自独立成框（输入框恢复四角圆角、
                 后缀补回左边框并居中），后缀里的路径无空格，靠 nowrap 也不会撑宽（它已独占一行）。 -->
            <div class="flex flex-col gap-2 md:flex-row md:gap-0">
              <Input
                id="oidc-callback-origin"
                v-model="form.callback_origin"
                class="rounded-r-none max-md:rounded-md"
                placeholder="http://localhost:3000"
                aria-describedby="oidc-callback-path"
              />
              <span
                id="oidc-callback-path"
                class="inline-flex shrink-0 items-center rounded-r-md border border-l-0 border-input bg-muted px-3 text-sm whitespace-nowrap text-muted-foreground max-md:justify-center max-md:rounded-md max-md:border-l max-md:py-3"
              >
                {{ OIDC_CALLBACK_PATH }}
              </span>
            </div>
          </div>

          <label class="flex max-lg:min-h-11 items-center gap-2">
            <input v-model="form.auto_provision" type="checkbox" class="size-4 accent-primary max-lg:size-5" />
            首次登录自动开通账号
          </label>

          <div class="flex flex-wrap items-center gap-2">
            <Button :disabled="saving" @click="submit">
              <Spinner v-if="saving" />
              保存
            </Button>
            <Button variant="outline" :disabled="probing || !form.issuer.trim()" @click="runProbe">
              <Spinner v-if="probing" />
              检测连通性
            </Button>
            <Button v-if="config.client_secret_set" variant="outline" @click="requestClearSecret(true)">
              清除密钥
            </Button>
          </div>

          <dl v-if="probeResult" class="grid gap-1 text-sm">
            <div class="flex gap-2">
              <dt class="shrink-0 text-muted-foreground">Issuer</dt>
              <dd class="min-w-0 break-all">{{ probeResult.issuer }}</dd>
            </div>
            <div class="flex gap-2">
              <dt class="shrink-0 text-muted-foreground">授权端点</dt>
              <dd class="min-w-0 break-all">{{ probeResult.authorization_endpoint }}</dd>
            </div>
            <div class="flex gap-2">
              <dt class="shrink-0 text-muted-foreground">令牌端点</dt>
              <dd class="min-w-0 break-all">{{ probeResult.token_endpoint }}</dd>
            </div>
            <div class="flex gap-2">
              <dt class="shrink-0 text-muted-foreground">JWKS</dt>
              <dd class="min-w-0 break-all">{{ probeResult.jwks_uri }}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <!-- 角色规则：未命中即拒绝登录 -->
      <Card>
        <CardHeader>
          <CardTitle class="text-base">角色规则</CardTitle>
        </CardHeader>
        <CardContent>
          <OidcRuleTable
            v-model:rules="form.role_rules"
            :targets="roleTargets"
            target-label="角色"
            empty-text="暂无规则，匹配不到规则的用户将被拒绝登录"
            :target-of="(r) => r.role_id"
            :make-rule="(expression, id) => ({ expression, role_id: id })"
            @update:rules="persistRules({ role_rules: $event })"
          />
        </CardContent>
      </Card>

      <!-- 部门规则：未命中不拒绝登录，只是不改动账号现有部门 -->
      <Card>
        <CardHeader>
          <CardTitle class="text-base">部门规则</CardTitle>
        </CardHeader>
        <CardContent>
          <OidcRuleTable
            v-model:rules="form.department_rules"
            :targets="departmentTargets"
            target-label="部门"
            empty-text="暂无规则，登录不会改变账号现有部门"
            :target-of="(r) => r.department_id"
            :make-rule="(expression, id) => ({ expression, department_id: id })"
            @update:rules="persistRules({ department_rules: $event })"
          />
        </CardContent>
      </Card>

      <!-- 规则验证 -->
      <Card>
        <CardHeader>
          <CardTitle class="text-base">规则验证</CardTitle>
        </CardHeader>
        <CardContent class="grid gap-3">
          <OidcClaimsPreview
            :role-rules="form.role_rules"
            :department-rules="form.department_rules"
            :roles="roleTargets"
            :departments="departmentTargets"
          />
        </CardContent>
      </Card>
    </template>

    <ConfirmDialog
      :open="!!clearSecretTarget"
      title="清除客户端密钥"
      confirm-text="清除"
      destructive
      :loading="clearingSecret"
      @update:open="onClearSecretOpenChange"
      @confirm="confirmClearSecret"
    />
  </PageShell>
</template>
