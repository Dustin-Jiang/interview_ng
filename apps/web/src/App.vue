<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { Toaster } from 'vue-sonner'
import {
  DoorOpen,
  Gavel,
  Home,
  LogOut,
  MonitorPlay,
  Settings,
  UsersRound,
  type LucideIcon,
} from 'lucide-vue-next'

import { useAuth, ensureAuthReady } from '@/composables/useAuth'
import { useTheme } from '@/composables/useTheme'
import { isNavPathActive } from '@/domain/nav'
import { PERMISSIONS } from '@/models'
import { hasAnyManagePermission } from '@/presenters/permissions'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { tabItemVariants } from '@/components/ui/tokens'
import ThemeToggle from '@/components/app/ThemeToggle.vue'
import { cn } from '@/lib/utils'

const route = useRoute()
const router = useRouter()
const { user, isLoggedIn, hasPermission, permissions, logout } = useAuth()
/** 实际生效主题：交给 vue-sonner 让提示条与全站一致（跟随系统时也以解析结果为准）。 */
const { resolved: themeResolved } = useTheme()

/** 导航项（图标供手机底部导航使用，桌面顶栏仍只显示文字，避免桌面观感受影响）。 */
const navItems = computed<{ name: string; to: { name: string }; icon: LucideIcon }[]>(() => {
  const items = [
    { name: '首页', to: { name: 'home' }, icon: Home },
    // 查看与管理分离：名册对所有人可见，管理入口仅对持权限者渲染。
    { name: '候选人', to: { name: 'candidates' }, icon: UsersRound },
    { name: '候场大屏', to: { name: 'board' }, icon: MonitorPlay },
    { name: '捡漏竞拍', to: { name: 'leftover' }, icon: Gavel },
  ]
  if (hasPermission(PERMISSIONS.ROOMS_VIEW)) {
    items.push({ name: '面试房间', to: { name: 'rooms' }, icon: DoorOpen })
  }
  // 管理功能整合进设置页（桌面左右分栏，手机为分区标签条）。
  if (hasAnyManagePermission(permissions.value)) {
    items.push({ name: '设置', to: { name: 'settings' }, icon: Settings })
  }
  return items
})

/** 导航项 + 解析后的目标路径（高亮按路径归属判定，路径取自路由表，不二次定义）。 */
const navLinks = computed(() =>
  navItems.value.map((item) => ({ ...item, path: router.resolve(item.to).path })),
)

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
  <!-- h-dvh：手机上按可视高度计算，软键盘弹出时内部滚动区自动收缩，输入框不被遮挡。 -->
  <div class="flex h-dvh flex-col overflow-hidden bg-background font-sans">
    <!-- 全局导航：登录后且非房间内部显示 -->
    <header
      v-if="showNav"
      class="z-40 w-full shrink-0 border-b bg-background/95 pt-[env(safe-area-inset-top)] backdrop-blur"
    >
      <div class="mx-auto flex h-14 max-w-content items-center gap-3 px-4">
        <!-- 品牌：点击回到欢迎页（手机上让位给底部导航，不占顶部空间） -->
        <RouterLink
          :to="{ name: 'home' }"
          class="hidden shrink-0 items-center rounded-md text-lg font-semibold tracking-tight transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring md:inline-flex max-lg:min-h-11"
        >
          面试系统 · 控制台
        </RouterLink>
        <!-- 桌面端：横向主导航（手机端改用底部导航，故此处隐藏） -->
        <nav class="hidden min-w-0 items-center gap-1 md:flex" aria-label="主导航">
          <RouterLink
            v-for="item in navLinks"
            :key="item.name"
            :to="item.to"
            :class="tabItemVariants({ active: isNavPathActive(route.path, item.path) })"
          >
            {{ item.name }}
          </RouterLink>
        </nav>
        <div class="ml-auto flex shrink-0 items-center gap-2">
          <!-- 用户标识：首字母头像 + 姓名（姓名在窄屏隐藏，避免与主导航抢空间把图标按钮压小于 44px） -->
          <IconBadge size="sm" class="text-xs font-semibold" :title="displayName" aria-hidden="true">
            {{ avatarChar }}
          </IconBadge>
          <span class="hidden max-w-[8rem] truncate text-sm text-muted-foreground lg:inline">{{ displayName }}</span>
          <ThemeToggle />
          <Button variant="ghost" size="icon" aria-label="退出登录" @click="logout">
            <LogOut aria-hidden="true" />
          </Button>
        </div>
      </div>
    </header>

    <!-- 主内容：全高 flex 布局，滚动在内部视图处理 -->
    <div class="min-h-0 flex-1">
      <RouterView />
    </div>

    <!-- 手机端底部导航：所有目的地一屏可达（顶栏横向滚动会藏起末尾项，实测「设置」曾整项不可见）。 -->
    <nav
      v-if="showNav"
      class="z-40 w-full shrink-0 border-t bg-background/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden"
      aria-label="主导航"
    >
      <ul class="flex items-stretch">
        <li v-for="item in navLinks" :key="item.name" class="min-w-0 flex-1">
          <RouterLink
            :to="item.to"
            :aria-current="isNavPathActive(route.path, item.path) ? 'page' : undefined"
            :class="
              cn(
                'flex h-14 min-w-0 flex-col items-center justify-center gap-1 px-0.5 text-[11px] leading-none transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
                isNavPathActive(route.path, item.path)
                  ? 'font-medium text-primary'
                  : 'text-muted-foreground',
              )
            "
          >
            <component :is="item.icon" class="h-5 w-5 shrink-0" aria-hidden="true" />
            <span class="max-w-full truncate">{{ item.name }}</span>
          </RouterLink>
        </li>
      </ul>
    </nav>

    <Toaster position="top-right" :theme="themeResolved" />
  </div>
</template>
