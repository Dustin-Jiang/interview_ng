<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { ChevronRight } from 'lucide-vue-next'

import { useAuth } from '@/composables/useAuth'
import { PERMISSIONS } from '@/models'
import { groupPermissionEntries, type PermissionGroup } from '@/presenters/permissions'
import { formatDateTime } from '@/lib/format'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { tileVariants } from '@/components/ui/tokens'
import PageShell from '@/components/app/PageShell.vue'
import { cn } from '@/lib/utils'

const { user, roles, permissions, hasPermission } = useAuth()

/** 我的权限按「管理/流程」分组有序展示。 */
const myPermGroups = computed<Record<PermissionGroup, string[]>>(() => groupPermissionEntries(permissions.value))
const hasAnyPermission = computed(() => myPermGroups.value['管理'].length > 0 || myPermGroups.value['流程'].length > 0)

/** 快捷入口：按权限显隐（查看类入口对所有人开放，管理类仅持权限者可见）。 */
const quickLinks = computed(() => {
  const links = [
    {
      name: '候选人',
      to: { name: 'candidates' },
      description: '名册与面试过程记录',
      visible: true,
    },
    {
      name: '候场大屏',
      to: { name: 'waiting' },
      description: '未完成名单与签到',
      visible: true,
    },
    {
      name: '面试房间',
      to: { name: 'room' },
      description: '进入房间拉取候选人、记录面试',
      visible: hasPermission(PERMISSIONS.ROOMS_VIEW),
    },
    {
      name: '候选人管理',
      to: { name: 'candidate-manage' },
      description: '签到、资料维护与状态重置',
      visible: hasPermission(PERMISSIONS.CANDIDATES_MANAGE),
    },
    {
      name: '面试官管理',
      to: { name: 'users' },
      description: '管理账号、角色与权限',
      visible: hasPermission(PERMISSIONS.USERS_MANAGE),
    },
  ]
  return links.filter((l) => l.visible)
})
</script>

<template>
  <PageShell :title="`你好，${user?.name || user?.username || '访客'}`" description="欢迎回来，祝面试工作顺利。">
    <div class="grid gap-4 lg:grid-cols-2">
      <!-- 个人信息 -->
      <Card>
        <CardHeader>
          <CardTitle>个人信息</CardTitle>
          <CardDescription>当前登录账号的基本资料。</CardDescription>
        </CardHeader>
        <CardContent class="space-y-2 text-sm">
          <div class="flex items-center justify-between gap-3">
            <span class="text-muted-foreground">用户名</span>
            <span class="font-mono">{{ user?.username ?? '-' }}</span>
          </div>
          <div class="flex items-center justify-between gap-3">
            <span class="text-muted-foreground">姓名</span>
            <span>{{ user?.name || '-' }}</span>
          </div>
          <div class="flex items-center justify-between gap-3">
            <span class="text-muted-foreground">用户 ID</span>
            <span class="font-mono">#{{ user?.id ?? '-' }}</span>
          </div>
          <div class="flex items-center justify-between gap-3">
            <span class="shrink-0 text-muted-foreground">角色</span>
            <span v-if="roles.length" class="flex flex-wrap justify-end gap-1">
              <Badge v-for="r in roles" :key="r" variant="secondary">{{ r }}</Badge>
            </span>
            <span v-else class="text-muted-foreground">-</span>
          </div>
          <div class="flex items-center justify-between gap-3">
            <span class="shrink-0 text-muted-foreground">创建时间</span>
            <time>{{ formatDateTime(user?.created_at) }}</time>
          </div>
        </CardContent>
      </Card>

      <!-- 我的权限 -->
      <Card>
        <CardHeader>
          <CardTitle>我的权限</CardTitle>
          <CardDescription>来自所属角色的功能权限。</CardDescription>
        </CardHeader>
        <CardContent class="space-y-3 text-sm">
          <template v-if="hasAnyPermission">
            <div v-for="g in ['管理', '流程'] as const" :key="g">
              <p class="mb-1.5 text-xs font-medium text-muted-foreground">{{ g }}</p>
              <div v-if="myPermGroups[g].length" class="flex flex-wrap gap-1">
                <Badge v-for="p in myPermGroups[g]" :key="p" variant="outline" class="font-mono text-xs">
                  {{ p }}
                </Badge>
              </div>
              <p v-else class="text-xs text-muted-foreground">无</p>
            </div>
          </template>
          <p v-else class="text-sm text-muted-foreground">
            当前账号暂无功能权限，请联系管理员分配角色。
          </p>
        </CardContent>
      </Card>
    </div>

    <!-- 快捷入口（tileVariants 统一可交互表面，含 hover/focus 反馈） -->
    <div v-if="quickLinks.length" class="space-y-2">
      <h2 class="text-sm font-medium text-muted-foreground">快捷入口</h2>
      <div class="grid gap-3 sm:grid-cols-3">
        <RouterLink
          v-for="link in quickLinks"
          :key="link.name"
          :to="link.to"
          :class="cn(tileVariants(), 'flex items-center justify-between gap-2 p-4')"
        >
          <div class="min-w-0">
            <p class="truncate text-sm font-medium">{{ link.name }}</p>
            <p class="truncate text-xs text-muted-foreground">{{ link.description }}</p>
          </div>
          <ChevronRight class="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        </RouterLink>
      </div>
    </div>
  </PageShell>
</template>
