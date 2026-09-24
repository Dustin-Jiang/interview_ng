<!--
  SettingsObservabilityView —— 「可观测性」设置分区（users.manage）：
  指标与日志（OTLP push → GreptimeDB）的运行时配置：保存即生效，无需重启进程。
  密码只写不读：界面只显示「是否已配置」，不回收明文（与登录认证的客户端密钥同口径）。
  保存结果回发 applied：false = 已落库但观测当前处于关闭/初始化失败态，会给出提示。
-->
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'

import { useConfirmAction } from '@/composables/useConfirmAction'
import { useObservabilitySettings } from '@/composables/useObservabilitySettings'
import { toastError } from '@/lib/toast'
import type { ObservabilityConfig, ObservabilityConfigPayload } from '@/models'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Spinner } from '@/components/ui/spinner'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import ListSkeleton from '@/components/app/ListSkeleton.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'

const { config, loading, saving, load, save } = useObservabilitySettings()

onMounted(() => void load())

const form = ref({
  enabled: false,
  endpoint: '',
  database: '',
  username: '',
  /** 草稿态密码：空串 = 不修改（清除走单独按钮）。 */
  password: '',
  service_name: '',
})

// 服务端配置为准：加载/保存后重填草稿（密码不回收，留空 = 不修改）。
watch(
  config,
  (c) => {
    if (!c) return
    form.value = {
      enabled: c.enabled,
      endpoint: c.endpoint,
      database: c.database,
      username: c.username,
      password: '',
      service_name: c.service_name,
    }
  },
  { immediate: true },
)

const passwordPlaceholder = computed(() =>
  config.value?.password_set ? '已配置（留空则不修改）' : 'Basic 认证密码（可为空）',
)

/** 由「已保存的配置」组装 PUT 体（PUT 是整体覆盖，缺省值即服务端现值；密码省略 = 保持）。 */
function payloadOf(cfg: ObservabilityConfig, overrides: Partial<ObservabilityConfigPayload>): ObservabilityConfigPayload {
  return {
    enabled: cfg.enabled,
    endpoint: cfg.endpoint,
    database: cfg.database,
    username: cfg.username,
    service_name: cfg.service_name,
    ...overrides,
  }
}

async function submit(): Promise<void> {
  const cfg = config.value
  if (!cfg) return
  const endpoint = form.value.endpoint.trim()
  if (form.value.enabled && !/^https?:\/\//.test(endpoint)) {
    toast.error('Endpoint 需以 http:// 或 https:// 开头，例如 http://greptime:4000')
    return
  }
  const payload = payloadOf(cfg, {
    enabled: form.value.enabled,
    endpoint,
    database: form.value.database.trim(),
    username: form.value.username.trim(),
    service_name: form.value.service_name.trim() || 'interview_ng',
  })
  // 省略该字段 = 保持原密码；清空请用「清除密码」。
  if (form.value.password) payload.password = form.value.password
  try {
    const applied = await save(payload)
    if (applied) {
      toast.success('已保存并生效')
    } else {
      toast.error('已保存，但观测推进失败，当前处于关闭状态（检查地址与账号密码）')
    }
  } catch (e) {
    toastError(e)
  }
}

const {
  target: clearPasswordTarget,
  loading: clearingPassword,
  request: requestClearPassword,
  onOpenChange: onClearPasswordOpenChange,
  confirm: confirmClearPassword,
} = useConfirmAction<true>({
  action: async () => {
    const cfg = config.value
    if (!cfg) throw new Error('配置尚未加载')
    await save(payloadOf(cfg, { password: '' }))
  },
  success: () => '密码已清除',
})
</script>

<template>
  <PageShell title="可观测性">
    <template #actions>
      <RefreshButton label="刷新可观测性配置" :loading="loading" @click="load" />
    </template>

    <ListSkeleton v-if="loading && !config" :rows="2" item-class="h-40 w-full rounded-xl" class="space-y-4" />

    <template v-else-if="config">
      <Card>
        <CardHeader>
          <CardTitle class="text-base">OTLP 推送（GreptimeDB）</CardTitle>
        </CardHeader>
        <CardContent class="grid gap-4">
          <div class="flex items-center justify-between gap-4">
            <Label for="obs-enabled">启用</Label>
            <input id="obs-enabled" v-model="form.enabled" type="checkbox" class="size-4 accent-primary max-lg:size-5" />
          </div>

          <div class="grid gap-2">
            <Label for="obs-endpoint">Endpoint</Label>
            <Input
              id="obs-endpoint"
              v-model="form.endpoint"
              placeholder="http://greptime:4000"
              :disabled="!form.enabled"
            />
          </div>

          <div class="grid gap-4 md:grid-cols-2">
            <div class="grid gap-2">
              <Label for="obs-database">数据库</Label>
              <Input id="obs-database" v-model="form.database" placeholder="interview_ng" :disabled="!form.enabled" />
            </div>
            <div class="grid gap-2">
              <Label for="obs-username">用户名</Label>
              <Input id="obs-username" v-model="form.username" placeholder="greptime" :disabled="!form.enabled" />
            </div>
            <div class="grid gap-2">
              <Label for="obs-password">密码</Label>
              <Input id="obs-password" v-model="form.password" type="password" :placeholder="passwordPlaceholder" :disabled="!form.enabled" />
            </div>
            <div class="grid gap-2">
              <Label for="obs-service-name">服务名</Label>
              <Input id="obs-service-name" v-model="form.service_name" placeholder="interview_ng" :disabled="!form.enabled" />
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <Button :disabled="saving" @click="submit">
              <Spinner v-if="saving" />
              保存
            </Button>
            <Button
              v-if="config.password_set"
              variant="outline"
              :disabled="saving"
              @click="requestClearPassword(true)"
            >
              清除密码
            </Button>
          </div>
        </CardContent>
      </Card>
    </template>

    <ConfirmDialog
      :open="!!clearPasswordTarget"
      title="清除 OTLP Basic 密码"
      confirm-text="清除"
      destructive
      :loading="clearingPassword"
      @update:open="onClearPasswordOpenChange"
      @confirm="confirmClearPassword"
    />
  </PageShell>
</template>
