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
 *
 * 滚动条**必须自带横截面尺寸**（垂直 `w-2.5`、水平 `h-2.5`）：reka 只给滚动条元素
 * `position: absolute` 与主轴方向的内联 insets，横截面尺寸由调用方决定；而 thumb 的内联
 * `width/height: var(--reka-scroll-area-thumb-*)` 里，横截面那个变量只定义在**另一个**方向
 * 的滚动条子树上（自定义属性只沿自身子树继承），故本方向取到未定义值 → `width/height` 回落
 * `auto`，再被 `flex-1`（flex-basis 0）吃掉 → thumb 横截面为 0px，滚动条整体不可见。
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
      class="flex w-2.5 touch-none select-none border-l border-l-transparent p-px transition-colors hover:bg-accent"
    >
      <ScrollAreaThumb class="relative flex-1 rounded-full bg-border" />
    </ScrollAreaScrollbar>
    <ScrollAreaScrollbar
      orientation="horizontal"
      class="flex h-2.5 touch-none select-none border-t border-t-transparent p-px transition-colors hover:bg-accent"
    >
      <ScrollAreaThumb class="relative flex-1 rounded-full bg-border" />
    </ScrollAreaScrollbar>
    <ScrollAreaCorner class="bg-background" />
  </ScrollAreaRoot>
</template>
