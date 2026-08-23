<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { ScrollAreaRootProps } from 'reka-ui'
import {
  ScrollAreaCorner,
  ScrollAreaRoot,
  ScrollAreaScrollbar,
  ScrollAreaThumb,
  ScrollAreaViewport,
} from 'reka-ui'

import { cn } from '@/lib/utils'

/**
 * ScrollArea —— 统一滚动容器（reka-ui ScrollArea）。
 * 收编列表/详情区原生 overflow 滚动条，视觉上对齐主题（thumb 使用 border 色）。
 */
const props = withDefaults(
  defineProps<
    ScrollAreaRootProps & {
      class?: HTMLAttributes['class']
      viewportClass?: HTMLAttributes['class']
    }
  >(),
  { type: 'auto' },
)
</script>

<template>
  <ScrollAreaRoot
    :type="props.type"
    :dir="props.dir"
    :scroll-hide-delay="props.scrollHideDelay"
    :class="cn('relative overflow-hidden', props.class)"
  >
    <ScrollAreaViewport :class="cn('h-full w-full', props.viewportClass)">
      <slot />
    </ScrollAreaViewport>
    <ScrollAreaScrollbar
      orientation="vertical"
      class="flex touch-none select-none border-l border-l-transparent p-px transition-colors hover:bg-accent"
    >
      <ScrollAreaThumb class="relative flex-1 rounded-full bg-border" />
    </ScrollAreaScrollbar>
    <ScrollAreaScrollbar
      orientation="horizontal"
      class="flex touch-none select-none border-t border-t-transparent p-px transition-colors hover:bg-accent"
    >
      <ScrollAreaThumb class="relative flex-1 rounded-full bg-border" />
    </ScrollAreaScrollbar>
    <ScrollAreaCorner class="bg-background" />
  </ScrollAreaRoot>
</template>
