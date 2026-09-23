<!--
  RosterList —— 左栏候选人名册（滚动区 + listbox + 骨架/错误/空态）。
  收编「候选人查看」与「捡漏竞拍」两侧逐字节相同的名册实现：
  整行可点；键盘只有一个 Tab 停靠点（选中项，roving tabindex），列表内切换候选人走全局 ←/→
  （`useRosterSelection`）。**不做 ↑/↓ 列表导航**——方向键留给页面控件（如捡漏页的报价步进）。
  徽章差异由 #badges 插槽交由调用方渲染（名册行只显示姓名 / 状态徽章 / 简介，不显示房间名）。
-->
<script setup lang="ts" generic="T extends { id: number; name: string; profile?: string | null }">
import { nextTick, ref, watch, type Component } from 'vue'

import { Avatar, AvatarFallback, avatarVariants } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import EmptyState from '@/components/app/EmptyState.vue'
import ErrorAlert from '@/components/app/ErrorAlert.vue'
import ListSkeleton from '@/components/app/ListSkeleton.vue'
import { initialsOf } from '@/lib/format'

const props = withDefaults(
  defineProps<{
    items: readonly T[]
    selectedId: number | null
    /** 首屏加载中（列表为空时展示骨架，刷新时不打断已有内容）。 */
    skeleton?: boolean
    /** 拉取失败文案（有值时替代列表展示，并提供重试）。 */
    error?: string | null
    /** 空态文案与图标。 */
    emptyText: string
    emptyIcon?: Component
    /** 空态动作按钮文案（如「清除筛选」），有值时展示。 */
    emptyActionLabel?: string
    /** listbox 无障碍标签。 */
    listLabel: string
    retryLabel?: string
  }>(),
  {
    skeleton: false,
    error: null,
    emptyIcon: undefined,
    emptyActionLabel: '',
    retryLabel: '重试',
  },
)

const emit = defineEmits<{
  /** 点击选中（调用方通常会切到详情视图）。 */
  select: [item: T]
  retry: []
  'empty-action': []
}>()

const rosterEl = ref<HTMLElement | null>(null)

/**
 * 选中变更（←/→ 切换、深链恢复、外部程序化选中）时把选中项滚入名册视野。
 * block: nearest——仅当项越出可视区才移动（已在视野内不跳动）；只滚动不移动焦点。
 */
watch(
  () => props.selectedId,
  (id) => {
    if (id == null) return
    void nextTick(() => {
      const idx = props.items.findIndex((c) => c.id === id)
      if (idx === -1) return
      const options = rosterEl.value?.querySelectorAll<HTMLElement>('[role="option"]')
      options?.[idx]?.scrollIntoView({ block: 'nearest' })
    })
  },
)
</script>

<template>
  <div
    ref="rosterEl"
    role="listbox"
    :aria-label="props.listLabel"
    class="min-h-0 flex-1 space-y-1 overflow-y-auto p-2"
  >
    <!-- 首屏骨架 -->
    <ListSkeleton
      v-if="props.skeleton"
      :rows="5"
      item-class="h-14 w-full rounded-md"
      class="p-2"
    />

    <!-- 拉取失败 -->
    <ErrorAlert
      v-else-if="props.error"
      variant="plain"
      :message="props.error"
      :retry-label="props.retryLabel"
      @retry="emit('retry')"
    />

    <!-- 空态 -->
    <EmptyState v-else-if="props.items.length === 0" bare :icon="props.emptyIcon" class="py-10">
      {{ props.emptyText }}
      <template v-if="props.emptyActionLabel" #action>
        <Button variant="outline" size="sm" @click="emit('empty-action')">
          {{ props.emptyActionLabel }}
        </Button>
      </template>
    </EmptyState>

    <!-- 列表项：整行可点；Tab 停靠选中项（roving tabindex），切换走 ←/→ 全局键 -->
    <button
      v-for="c in props.items"
      :key="c.id"
      type="button"
      role="option"
      :aria-selected="c.id === props.selectedId"
      :tabindex="c.id === props.selectedId ? 0 : -1"
      class="flex w-full cursor-pointer items-center gap-3 rounded-md p-3 text-left transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      :class="c.id === props.selectedId ? 'bg-accent' : ''"
      @click="emit('select', c)"
    >
      <Avatar :class="avatarVariants({ size: 'sm' })" aria-hidden="true">
        <AvatarFallback>{{ initialsOf(c.name) }}</AvatarFallback>
      </Avatar>
      <span class="min-w-0 flex-1">
        <span class="flex items-center justify-between gap-2">
          <span class="min-w-0 truncate text-sm font-medium">{{ c.name }}</span>
          <!-- 徽章插槽：候选人状态 / 本部门录取决定 / 出价与成交结果。
               允许收缩与换行（长部门名 + 多枚徽章时不再把整行撑宽）。 -->
          <span v-if="$slots.badges" class="flex min-w-0 flex-wrap items-center justify-end gap-1">
            <slot name="badges" :item="c" />
          </span>
        </span>
        <span class="mt-0.5 block truncate text-xs text-muted-foreground">{{ c.profile || '无简介' }}</span>
      </span>
    </button>
  </div>
</template>
