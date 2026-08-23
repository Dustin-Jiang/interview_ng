<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { ClipboardList } from 'lucide-vue-next'

import { useAuth } from '@/composables/useAuth'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { IconBadge } from '@/components/ui/icon-badge'
import { Spinner } from '@/components/ui/spinner'

const router = useRouter()
const { login } = useAuth()

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
</script>

<template>
  <div class="flex h-full items-center justify-center bg-muted/30 p-4">
    <Card class="w-full max-w-sm">
      <CardHeader class="items-center text-center">
        <!-- 品牌标识：图标 + 标题 -->
        <IconBadge size="lg" tone="solid" class="mx-auto mb-1" aria-hidden="true">
          <ClipboardList />
        </IconBadge>
        <CardTitle>面试系统 · 控制台</CardTitle>
      </CardHeader>
      <CardContent class="grid gap-4">
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
