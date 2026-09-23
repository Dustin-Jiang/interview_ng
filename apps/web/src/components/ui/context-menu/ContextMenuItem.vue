<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { ContextMenuItemProps } from 'reka-ui'
import { computed } from 'vue'
import { ContextMenuItem } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<ContextMenuItemProps & { class?: HTMLAttributes['class']; destructive?: boolean }>(),
  { destructive: false },
)

/**
 * 只把 reka-ui 认识的 props 透传下去：`destructive` 是本组件的视觉开关，
 * 不是菜单项属性，落到 DOM 上会变成无意义的属性。
 */
const forwarded = computed(() => {
  const { class: _class, destructive: _destructive, ...rest } = props
  return rest
})
</script>

<template>
  <!-- 菜单项：与 ui/select 的条目同一套焦点/禁用样式；触屏下抬到 ≥44px。
       destructive 用**实底**表达（与 ConfirmDialog 的 destructive 按钮同一语言）：
       `--destructive` 在深色下是背景级的暗红（0 62.8% 30.6%），拿它当文字色几乎看不见，
       会被误读成「禁用」；填入白字后两种主题都清晰。 -->
  <ContextMenuItem
    v-bind="forwarded"
    :class="
      cn(
        'relative flex w-full cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none focus:bg-accent focus:text-accent-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 max-lg:min-h-11',
        props.destructive && 'focus:bg-destructive focus:text-destructive-foreground',
        props.class,
      )
    "
  >
    <slot />
  </ContextMenuItem>
</template>
