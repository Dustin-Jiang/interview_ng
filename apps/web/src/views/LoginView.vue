<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { ClipboardList } from 'lucide-vue-next'

import { OIDC_AUTHORIZATION_PATH } from '@/api/http'
import { useAuth } from '@/composables/useAuth'
import { oidcErrorMessage } from '@/domain/oidc'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { IconBadge } from '@/components/ui/icon-badge'
import { Spinner } from '@/components/ui/spinner'
import ThemeToggle from '@/components/app/ThemeToggle.vue'

const route = useRoute()
const router = useRouter()
const { login, oidcEnabled, loadAuthenticationOptions, completeOidc } = useAuth()

const username = ref('')
const password = ref('')
const submitting = ref(false)

async function submit() {
  if (!username.value.trim() || !password.value) {
    toast.error('请输入用户名与密码')
    return
  }
  if (submitting.value) return
  submitting.value = true
  try {
    await login(username.value.trim(), password.value)
    toast.success('登录成功')
    void router.push({ name: 'home' })
  } catch (e) {
    toast.error((e as Error).message || '登录失败')
  } finally {
    submitting.value = false
  }
}

/** 统一身份认证：整页跳转到后端授权入口（非 XHR）。 */
function toOidc() {
  window.location.assign(OIDC_AUTHORIZATION_PATH)
}

// 回调落地：oidc_code 换会话；oidc_error 展示原因并清掉 query（避免刷新重复提示）。
onMounted(async () => {
  void loadAuthenticationOptions()
  const code = route.query.oidc_code
  if (typeof code === 'string' && code) {
    submitting.value = true
    try {
      await completeOidc(code)
      toast.success('登录成功')
      await router.replace({ name: 'home' })
    } catch (e) {
      toast.error((e as Error).message || '统一身份认证登录失败')
    } finally {
      submitting.value = false
    }
    return
  }
  const error = route.query.oidc_error
  if (typeof error === 'string' && error) {
    toast.error(oidcErrorMessage(error))
    void router.replace({ name: 'login' })
  }
})
</script>

<template>
  <div class="relative flex h-full items-center justify-center bg-muted/30 p-4">
    <!-- 登录前也要能切主题（顶栏此时不可见） -->
    <div class="absolute right-4 top-4">
      <ThemeToggle />
    </div>
    <Card class="w-full max-w-sm">
      <CardHeader class="items-center text-center">
        <!-- 品牌标识：图标 + 标题 -->
        <IconBadge size="lg" tone="solid" class="mx-auto mb-1" aria-hidden="true">
          <ClipboardList />
        </IconBadge>
        <CardTitle>面试系统 · 控制台</CardTitle>
      </CardHeader>
      <CardContent class="grid gap-4">
        <template v-if="oidcEnabled">
          <Button variant="outline" aria-label="统一身份认证登录" @click="toOidc">
            统一身份认证登录
          </Button>
          <Separator />
        </template>
        <div class="grid gap-2">
          <Label for="login-username">用户名</Label>
          <Input
            id="login-username"
            v-model="username"
            autocomplete="username"
            placeholder="用户名"
            autofocus
            @keydown.enter="submit"
          />
        </div>
        <div class="grid gap-2">
          <Label for="login-password">密码</Label>
          <Input
            id="login-password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            placeholder="密码"
            @keydown.enter="submit"
          />
        </div>
        <Button :disabled="submitting" aria-label="登录" @click="submit">
          <Spinner v-if="submitting" />
          {{ submitting ? '登录中…' : '登录' }}
        </Button>
      </CardContent>
    </Card>
  </div>
</template>
