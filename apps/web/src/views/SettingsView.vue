<!--
  SettingsView —— 设置页外壳：左右分栏。
  左侧：分区导航（候选人管理 / 面试官 / 角色，按权限显隐）；
  右侧：嵌套路由渲染当前分区内容。
  无任一管理权限时重定向回首页；直接访问无权分区时跳到首个可见分区。
-->
<script setup lang="ts">
import { computed, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { Building2, ShieldCheck, UserCog, UsersRound } from 'lucide-vue-next'

import { useAuth } from '@/composables/useAuth'
import { PERMISSIONS } from '@/models'
import { navItemVariants } from '@/components/ui/tokens'
import { cn } from '@/lib/utils'

const route = useRoute()
const router = useRouter()
const { hasPermission } = useAuth()

/** 设置页分区（按权限显隐，顺序即侧栏展示顺序）。 */
const sections = computed(() => {
  const items: { name: string; label: string; icon: typeof UsersRound }[] = []
  const canManageCandidates =
    hasPermission(PERMISSIONS.CANDIDATES_MANAGE) ||
    hasPermission(PERMISSIONS.CANDIDATES_CREATE) ||
    hasPermission(PERMISSIONS.CANDIDATES_CHECKIN)
  if (canManageCandidates) {
    items.push({ name: 'settings-candidates', label: '候选人管理', icon: UsersRound })
  }
  if (hasPermission(PERMISSIONS.USERS_MANAGE)) {
    items.push({ name: 'settings-users', label: '面试官', icon: UserCog })
    items.push({ name: 'settings-roles', label: '角色', icon: ShieldCheck })
    items.push({ name: 'settings-departments', label: '部门', icon: Building2 })
  }
  return items
})

/** 当前路由是否为可见分区之一。 */
const currentAllowed = computed(() => sections.value.some((s) => s.name === route.name))

/** 无任何管理权限 → 首页；否则兜底跳首个可见分区。 */
watch(
  currentAllowed,
  (allowed) => {
    if (allowed) return
    const first = sections.value[0]
    void router.replace(first ? { name: first.name } : { name: 'home' })
  },
  { immediate: true },
)
</script>

<template>
  <!-- 整体限制最大宽度并居中（max-w-content 语义档位） -->
  <div class="mx-auto flex h-full w-full max-w-content overflow-hidden">
    <!-- 左侧：分区导航 -->
    <aside
      class="flex w-52 shrink-0 flex-col"
      aria-label="设置分区导航"
    >
      <nav class="flex flex-col gap-1 p-3">
        <RouterLink
          v-for="s in sections"
          :key="s.name"
          :to="{ name: s.name }"
          :class="cn(navItemVariants({ active: route.name === s.name }))"
        >
          <component :is="s.icon" class="h-4 w-4 shrink-0" aria-hidden="true" />
          {{ s.label }}
        </RouterLink>
      </nav>
    </aside>

    <!-- 右侧：当前分区内容 -->
    <main class="min-h-0 flex-1">
      <RouterView />
    </main>
  </div>
</template>
