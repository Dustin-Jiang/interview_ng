<!--
  SettingsAuthenticationView —— 「登录认证」设置分区（users.manage）：
  单点登录（OIDC）连接参数（开关 / Issuer / 客户端 / Scopes / 回调地址 / 自动开通 + 连通性检测）
  + 「组 → 角色」JMESPath 规则表（自上而下首个命中生效）
  + 规则验证（粘贴 ID token 声明试算命中结果）。
  客户端密钥只写不读：界面只显示「是否已配置」，不回收明文。
-->
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'

import { useConfirmAction } from '@/composables/useConfirmAction'
import { useOidcSettings } from '@/composables/useOidcSettings'
import { toastError } from '@/lib/toast'
import type { OidcConfigPayload, OidcRulePayload } from '@/models'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import ErrorAlert from '@/components/app/ErrorAlert.vue'
import ListSkeleton from '@/components/app/ListSkeleton.vue'
import OidcClaimsPreview from '@/components/app/OidcClaimsPreview.vue'
import OidcRoleRules from '@/components/app/OidcRoleRules.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'

const { config, roles, loading, error, saving, probing, probeResult, load, save, probe, clearSecret } =
  useOidcSettings()

onMounted(() => void load())

/** 默认回调地址：与后端同源（开发期经 vite 代理到 :8080）。 */
function defaultRedirectURL(): string {
  return `${window.location.origin}/api/oidc/callback`
}

const form = ref({
  enabled: false,
  issuer: '',
  client_id: '',
  /** 草稿态密钥：空串 = 不修改（清除走单独按钮）。 */
  client_secret: '',
  scopes: 'openid profile email',
  redirect_url: '',
  auto_provision: true,
  rules: [] as OidcRulePayload[],
})

// 服务端配置为准：加载/保存后重填草稿（首次回调地址为空时给出默认值）。
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
      redirect_url: c.redirect_url || defaultRedirectURL(),
      auto_provision: c.auto_provision,
      rules: c.rules.map((r) => ({ expression: r.expression, role_id: r.role_id })),
    }
  },
  { immediate: true },
)

const secretPlaceholder = computed(() =>
  config.value?.client_secret_set ? '已配置（留空则不修改）' : '客户端密钥（公开客户端可留空）',
)

async function submit(): Promise<void> {
  const payload: OidcConfigPayload = {
    enabled: form.value.enabled,
    issuer: form.value.issuer.trim(),
    client_id: form.value.client_id.trim(),
    scopes: form.value.scopes.trim(),
    redirect_url: form.value.redirect_url.trim(),
    auto_provision: form.value.auto_provision,
    rules: form.value.rules,
  }
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

    <ErrorAlert v-if="error" :message="`配置加载失败：${error}`" retry-label="重试" @retry="load" />

    <ListSkeleton v-if="loading && !config" :rows="2" item-class="h-40 w-full rounded-xl" class="space-y-4" />

    <template v-else-if="config">
      <!-- 连接参数 -->
      <Card>
        <CardHeader>
          <CardTitle class="text-base">单点登录（OIDC）</CardTitle>
        </CardHeader>
        <CardContent class="grid gap-4">
          <div class="flex items-center gap-2">
            <input id="oidc-enabled" v-model="form.enabled" type="checkbox" class="size-4 accent-primary" />
            <Label for="oidc-enabled">启用统一身份认证</Label>
          </div>

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
            <Label for="oidc-redirect">回调地址</Label>
            <Input id="oidc-redirect" v-model="form.redirect_url" :placeholder="defaultRedirectURL()" />
          </div>

          <div class="flex items-center gap-2">
            <input id="oidc-auto-provision" v-model="form.auto_provision" type="checkbox" class="size-4 accent-primary" />
            <Label for="oidc-auto-provision">首次登录自动开通账号</Label>
          </div>

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
              <dd class="break-all">{{ probeResult.issuer }}</dd>
            </div>
            <div class="flex gap-2">
              <dt class="shrink-0 text-muted-foreground">授权端点</dt>
              <dd class="break-all">{{ probeResult.authorization_endpoint }}</dd>
            </div>
            <div class="flex gap-2">
              <dt class="shrink-0 text-muted-foreground">令牌端点</dt>
              <dd class="break-all">{{ probeResult.token_endpoint }}</dd>
            </div>
            <div class="flex gap-2">
              <dt class="shrink-0 text-muted-foreground">JWKS</dt>
              <dd class="break-all">{{ probeResult.jwks_uri }}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <!-- 角色规则 -->
      <Card>
        <CardHeader>
          <CardTitle class="text-base">角色规则</CardTitle>
        </CardHeader>
        <CardContent>
          <OidcRoleRules v-model:rules="form.rules" :roles="roles" />
        </CardContent>
      </Card>

      <!-- 规则验证 -->
      <Card>
        <CardHeader>
          <CardTitle class="text-base">规则验证</CardTitle>
        </CardHeader>
        <CardContent class="grid gap-3">
          <OidcClaimsPreview :rules="form.rules" :roles="roles" />
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
