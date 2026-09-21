<!--
  RosterList —— 左栏候选人名册（滚动区 + listbox + 骨架/错误/空态）。
  收编「候选人查看」与「捡漏竞拍」两侧逐字节相同的名册实现：
  整行可点、roving tabindex（仅选中项可 Tab）、↑/↓ 在列表内移动并回填焦点。
  徽章与元信息差异由 #badges / #meta 插槽交由调用方渲染。
-->
<script setup lang="ts" generic="T extends { id: number; name: string; profile?: string | null }">
import { nextTick, ref, type Component } from 'vue'

import { Avatar, AvatarFallback, avatarVariants } from '@/components/ui/avatar'
import { ScrollArea } from '@/components/ui/scroll-area'
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
  /** 键盘高亮移动（仅移动选中高亮，不切换移动端视图）。 */
  highlight: [item: T]
  retry: []
  'empty-action': []
}>()

const rosterEl = ref<HTMLElement | null>(null)

/** ↑/↓ 移动选中并回填焦点（roving tabindex）。 */
function onKeydown(e: KeyboardEvent) {
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
  e.preventDefault()
  const list = props.items
  const idx = list.findIndex((c) => c.id === props.selectedId)
  if (idx === -1) return
  const next = list[idx + (e.key === 'ArrowDown' ? 1 : -1)]
  if (!next) return
  emit('highlight', next)
  void nextTick(() => {
    const options = rosterEl.value?.querySelectorAll<HTMLElement>('[role="option"]')
    options?.[list.findIndex((c) => c.id === next.id)]?.focus()
  })
}
</script>

<template>
  <ScrollArea class="min-h-0 flex-1">
    <div ref="rosterEl" role="listbox" :aria-label="props.listLabel" class="space-y-1 p-2" @keydown="onKeydown">
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

      <!-- 列表项：整行可点、键盘可达（roving tabindex + 方向键） -->
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
            <!-- 徽章插槽：候选人状态 / 本部门录取决定 / 出价与成交结果 -->
            <span v-if="$slots.badges" class="flex shrink-0 items-center gap-1">
              <slot name="badges" :item="c" />
            </span>
          </span>
          <span class="mt-0.5 flex items-center justify-between gap-2 text-xs text-muted-foreground">
            <span class="min-w-0 truncate">{{ c.profile || '无简介' }}</span>
            <!-- 元信息插槽：房间号等 -->
            <slot name="meta" :item="c" />
          </span>
        </span>
      </button>
    </div>
  </ScrollArea>
</template>
