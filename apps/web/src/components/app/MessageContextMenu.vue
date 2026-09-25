<!--
  MessageContextMenu —— 一条面试记录的右键菜单内容（自 `MessageTranscript` 拆出：主组件已接近 400 行上限）。
  行为、`role`/`aria-*` 属性与 class 与拆分前逐字一致，只搬位置；纯展示 + 抛事件，
  数据（能否回表情 / 能否编辑 / 已回的表情 / 表情明细）由持有方算好传入，写入动作一律回抛。
  **必须渲染在 `ContextMenu` 根之内**（reka 靠 provide/inject 关联内容与触发区，跨组件边界无碍）。
-->
<script setup lang="ts">
import { computed } from 'vue'
import { Copy, Pencil, Quote, Undo2 } from 'lucide-vue-next'

import { REACTION_EMOJIS } from '@/domain/messages'

import {
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
} from '@/components/ui/context-menu'

const props = defineProps<{
  /** 能否回表情（`rooms.chat` 且归属候选人）。 */
  reactable: boolean
  /** 能否编辑/撤回（本人且两分钟内）。 */
  modifiable: boolean
  /** 能否写记录（引用入口，与表情同属 `rooms.chat`）。 */
  canChat: boolean
  /** 我在本条上已回过的表情（网格里标出，避免误点成撤回）。 */
  mine: Set<string>
  /** 「谁回了什么」明细（按表情一行，空数组表示没有）。 */
  details: { emoji: string; count: number; names: string }[]
}>()

const emit = defineEmits<{
  /** 开关我在某个表情上的回复。 */
  react: [emoji: string]
  copy: []
  quote: []
  edit: []
  recall: []
}>()

/** 有没有可展示的表情明细（同时决定分隔线与明细区是否渲染）。 */
const hasDetails = computed(() => props.details.length > 0)
</script>

<template>
  <!-- 弹层默认 min-w 只有 9rem：6 列表情网格需要更宽的底板，顺带让「谁回了什么」一行放得下。
       触屏再放宽到 17rem——手机窄屏下 13rem 会把每格表情压成 30px 宽的细条（高度却是 44px），
       「过细」得不像可点的目标；17rem 让 6 列各约 42px ≈ 方格。上限 `100vw-1rem` 防溢出。 -->
  <ContextMenuContent
    class="max-h-[var(--reka-context-menu-content-available-height)] min-w-[13rem] max-w-[calc(100vw-1rem)] overflow-y-auto max-lg:min-w-[17rem]"
  >
    <!-- 表情回复：24 个表情铺成 6×4 网格（与 `REACTION_EMOJIS` 的分组顺序一致，一点即回，
         不必再开子菜单）。
         已经回过的表情用实底标出（MenuItem 的 aria-checked + data-state，见下），避免「再点一下把它撤了」的意外。 -->
    <div v-if="props.reactable" class="grid grid-cols-6 gap-0.5 p-1" role="presentation">
      <ContextMenuItem
        v-for="e in REACTION_EMOJIS"
        :key="e"
        class="justify-center px-0 text-base leading-none"
        :class="props.mine.has(e) ? 'bg-primary text-primary-foreground' : ''"
        role="menuitemcheckbox"
        :aria-checked="props.mine.has(e)"
        :aria-label="props.mine.has(e) ? `撤回 ${e}` : `用 ${e} 回复`"
        @select="emit('react', e)"
      >
        <span aria-hidden="true">{{ e }}</span>
      </ContextMenuItem>
    </div>
    <ContextMenuSeparator v-if="props.reactable && (hasDetails || props.modifiable)" />

    <!-- 谁回了什么：每个表情一行，列出回复人（姓名·部门，自己显示「我」）。
         纯展示区，不是菜单项——避免方向键停在无动作的行上。 -->
    <div v-if="hasDetails" class="space-y-0.5 px-2 py-1.5" role="group" aria-label="表情回复明细">
      <div v-for="d in props.details" :key="d.emoji" class="flex items-baseline gap-2 text-xs">
        <!-- 与右侧姓名同字号、同基线：emoji 用更大的字号或 items-start 会让两者差开 ~2px -->
        <span class="w-4 shrink-0 text-center" aria-hidden="true">{{ d.emoji }}</span>
        <span class="shrink-0 tabular-nums text-muted-foreground">×{{ d.count }}</span>
        <span class="min-w-0 break-words text-foreground/90">{{ d.names }}</span>
      </div>
    </div>
    <ContextMenuSeparator v-if="hasDetails && props.modifiable" />

    <!-- 复制消息：任何能打开菜单的记录都可复制（含别人的、已过编辑窗口的）。 -->
    <ContextMenuItem @select="emit('copy')">
      <Copy class="h-4 w-4" aria-hidden="true" />
      复制消息
    </ContextMenuItem>
    <!-- 引用：可写记录（rooms.chat）即可引用任何一条先行消息，不受本人/时间窗口限制。
         图标与引用块里那枚 `Quote` 同一枚（引用这一动作在界面上只有一个符号）。 -->
    <ContextMenuItem v-if="props.canChat" @select="emit('quote')">
      <Quote class="h-4 w-4" aria-hidden="true" />
      引用
    </ContextMenuItem>
    <ContextMenuItem v-if="props.modifiable" @select="emit('edit')">
      <Pencil class="h-4 w-4" aria-hidden="true" />
      编辑
    </ContextMenuItem>
    <ContextMenuSeparator v-if="props.modifiable" />
    <ContextMenuItem v-if="props.modifiable" destructive @select="emit('recall')">
      <Undo2 class="h-4 w-4" aria-hidden="true" />
      撤回
    </ContextMenuItem>
  </ContextMenuContent>
</template>
