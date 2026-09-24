<!--
  SettingsView —— 设置页外壳：桌面左右分栏，手机顶部标签条。
  ≥md：左侧分区导航（候选人管理 / 数据导入 / 面试官 / 角色 / 部门 / 系统状态，按权限显隐）+ 右侧嵌套路由；
  <md：固定 208px 左栏会把主区挤到 ~120px，故改为顶部横向滑动的分区标签条，主区占满整宽。
  无任一管理权限时重定向回首页；直接访问无权分区时跳到首个可见分区。
-->
<script setup lang="ts">
import { computed, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { Activity, Building2, FileSpreadsheet, Gauge, KeyRound, ShieldCheck, UserCog, UsersRound } from 'lucide-vue-next'

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
  if (hasPermission(PERMISSIONS.CANDIDATES_MANAGE)) {
    // 导入会覆盖既有候选人资料，故与「编辑候选人」同权限门槛。
    items.push({ name: 'settings-imports', label: '数据导入', icon: FileSpreadsheet })
  }
  if (hasPermission(PERMISSIONS.USERS_MANAGE)) {
    items.push({ name: 'settings-authentication', label: '登录认证', icon: KeyRound })
    items.push({ name: 'settings-users', label: '面试官', icon: UserCog })
    items.push({ name: 'settings-roles', label: '角色', icon: ShieldCheck })
    items.push({ name: 'settings-departments', label: '部门', icon: Building2 })
    items.push({ name: 'settings-system-status', label: '系统状态', icon: Gauge })
    items.push({ name: 'settings-observability', label: '可观测性', icon: Activity })
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
  <!-- 整体限制最大宽度并居中（max-w-content 语义档位）；手机上分区导航置顶、内容占满整宽 -->
  <div class="mx-auto flex h-full w-full max-w-content flex-col overflow-hidden md:flex-row">
    <!-- <md：顶部分区标签条（横向滑动，7 个分区滑得到最后一个；行高由 navItemVariants 抬到 44px） -->
    <nav
      class="flex shrink-0 items-center gap-1 overflow-x-auto border-b p-2 md:hidden"
      aria-label="设置分区导航"
    >
      <RouterLink
        v-for="s in sections"
        :key="s.name"
        :to="{ name: s.name }"
        :class="cn(navItemVariants({ active: route.name === s.name }), 'w-auto shrink-0 whitespace-nowrap')"
      >
        <component :is="s.icon" class="h-4 w-4 shrink-0" aria-hidden="true" />
        {{ s.label }}
      </RouterLink>
    </nav>

    <!-- ≥md：左侧固定宽度分区导航（手机上隐藏，改由上方标签条承载） -->
    <aside
      class="hidden w-52 shrink-0 flex-col md:flex"
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

    <!-- 右侧：当前分区内容（min-w-0：允许主区收缩到可用宽度，超宽表格由自身容器横向滚动，
         否则 flex 项的自动最小宽度会把外壳撑宽、被 overflow-hidden 直接裁掉）。 -->
    <main class="min-h-0 min-w-0 flex-1">
      <RouterView />
    </main>
  </div>
</template>
