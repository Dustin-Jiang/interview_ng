<!--
  MessageReactions —— 一条记录上的表情胶囊行（参照 shadcn `BubbleReactions`）。
  贴在气泡下沿：每个表情一枚胶囊（含计数），点一下开关「我自己的」这个表情；
  超过 4 个折叠成「+N」。已回的用实底 primary 表达（与页签激活态同一语言），
  未回的用 `bg-muted` + `foreground/20` 描边（深色下 `--border` 与 `--muted` 同色，不能只靠 border 勾边）。
  **触屏高度取 32px（`max-lg:min-h-8`）而不是全站约定的 44px**：胶囊是「emoji + 计数」这类内联小控件，
  44px 会让每颗胶囊被拉成近乎方形的高块（视觉上比气泡还厚重，实测 44px vs 桌面 22px），
  横向再靠 `max-lg:px-3.5` 拉宽成正常药丸形；32px 与 Material Chip 同档，命中区足够。
  纯展示 + 抛事件：写入由持有方负责（服务端权威）。
-->
<script setup lang="ts">
import type { ReactionSummary } from '@/domain/messages'

const props = defineProps<{
  /** 按表情聚合的回复（顺序稳定：调色板顺序）。 */
  summary: ReactionSummary[]
  /** 能否回复（无 rooms.chat 时只读展示）。 */
  reactable: boolean
  /** 写入中：防连点。 */
  busy: boolean
}>()

defineEmits<{ toggle: [emoji: string] }>()

/** 气泡下最多铺这么多个胶囊，其余折叠成「+N」——避免长条压到下一行。 */
const MAX_CHIPS = 4

const shown = () => props.summary.slice(0, MAX_CHIPS)
const hiddenCount = () => Math.max(0, props.summary.length - MAX_CHIPS)
</script>

<template>
  <div class="flex flex-wrap items-center gap-2" role="group" :aria-label="`表情回复：${summary.length} 种`">
    <button
      v-for="r in shown()"
      :key="r.emoji"
      type="button"
      class="flex cursor-pointer items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition-colors [transition-duration:var(--duration-quick)] max-lg:min-h-8 max-lg:gap-1.5 max-lg:px-3.5"
      :class="
        r.mine
          ? 'border-primary bg-primary font-medium text-primary-foreground'
          : 'border-foreground/20 bg-muted hover:border-foreground/40 active:bg-accent'
      "
      :disabled="!reactable || busy"
      :aria-pressed="r.mine"
      :aria-label="`${r.emoji} ${r.count} 人${r.mine ? '（含我，点击撤回）' : '（点击回复）'}`"
      @click="$emit('toggle', r.emoji)"
    >
      <span aria-hidden="true">{{ r.emoji }}</span>
      <span v-if="r.count > 1" class="tabular-nums opacity-80">{{ r.count }}</span>
    </button>
    <span
      v-if="hiddenCount() > 0"
      class="inline-flex items-center rounded-full border border-foreground/20 bg-muted px-2 py-0.5 text-xs text-muted-foreground max-lg:min-h-8 max-lg:px-3.5"
      aria-hidden="true"
    >
      +{{ hiddenCount() }}
    </span>
  </div>
</template>
