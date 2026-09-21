<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { Toaster } from 'vue-sonner'
import { LogOut } from 'lucide-vue-next'

import { useAuth, ensureAuthReady } from '@/composables/useAuth'
import { PERMISSIONS } from '@/models'
import { hasAnyManagePermission } from '@/presenters/permissions'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { tabItemVariants } from '@/components/ui/tokens'

const route = useRoute()
const { user, isLoggedIn, hasPermission, permissions, logout } = useAuth()

const navItems = computed(() => {
  const items = [
    { name: '首页', to: { name: 'home' } },
    // 查看与管理分离：名册对所有人可见，管理入口仅对持权限者渲染。
    { name: '候选人', to: { name: 'candidates' } },
    { name: '候场大屏', to: { name: 'waiting' } },
    { name: '捡漏竞拍', to: { name: 'leftover' } },
  ]
  if (hasPermission(PERMISSIONS.ROOMS_VIEW)) {
    items.push({ name: '面试房间', to: { name: 'room' } })
  }
  // 管理功能整合进设置页（左右分栏）。
  if (hasAnyManagePermission(permissions.value)) {
    items.push({ name: '设置', to: { name: 'settings' } })
  }
  return items
})

/** 用户展示名与头像首字符。 */
const displayName = computed(() => user.value?.name || user.value?.username || '')
const avatarChar = computed(() => displayName.value.slice(0, 1).toUpperCase())

/** 登录页隐藏全局导航（房间内也显示全局顶栏）。 */
const showNav = computed(() => isLoggedIn.value)

onMounted(() => {
  void ensureAuthReady()
})
</script>

<template>
  <div class="flex h-screen flex-col overflow-hidden bg-background font-sans">
    <!-- 全局导航：登录后且非房间内部显示 -->
    <header
      v-if="showNav"
      class="z-40 w-full shrink-0 border-b bg-background/95 backdrop-blur"
    >
      <div class="mx-auto flex h-14 max-w-content items-center gap-4 px-4">
        <!-- 品牌：点击回到欢迎页 -->
        <RouterLink
          :to="{ name: 'home' }"
          class="hidden shrink-0 rounded-md text-lg font-semibold tracking-tight transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:inline"
        >
          面试系统 · 控制台
        </RouterLink>
        <!-- 小屏下导航可横向滚动，避免溢出换行 -->
        <nav class="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto md:flex-none" aria-label="主导航">
          <RouterLink
            v-for="item in navItems"
            :key="item.name"
            :to="item.to"
            :class="tabItemVariants({ active: route.name === item.to.name })"
          >
            {{ item.name }}
          </RouterLink>
        </nav>
        <div class="ml-auto flex shrink-0 items-center gap-2">
          <!-- 用户标识：首字母头像 + 姓名 -->
          <IconBadge size="sm" class="text-xs font-semibold" :title="displayName" aria-hidden="true">
            {{ avatarChar }}
          </IconBadge>
          <span class="max-w-[8rem] truncate text-sm text-muted-foreground">{{ displayName }}</span>
          <Button variant="ghost" size="icon" aria-label="退出登录" @click="logout">
            <LogOut class="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      </div>
    </header>

    <!-- 主内容：全高 flex 布局，滚动在内部视图处理 -->
    <div class="min-h-0 flex-1">
      <RouterView />
    </div>

    <Toaster position="top-right" />
  </div>
</template>
