<!--
  ListSkeleton —— 列表首屏加载骨架。
  收编各视图重复的「aria-busy 容器 + N 行 Skeleton」实现；行数、条目高度与
  容器间距（space-y-2 / space-y-4）由 props 传入，网格列表用 layout="grid"。
-->
<script setup lang="ts">
import type { HTMLAttributes } from 'vue'

import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    /** 骨架行数。 */
    rows?: number
    /** 单行骨架尺寸档（如 h-12 rounded-md / h-16 rounded-md / h-[104px] rounded-xl）。 */
    itemClass?: string
    /** 排列方式：竖排列表 / 响应式网格（房间卡片）。 */
    layout?: 'stack' | 'grid'
    class?: HTMLAttributes['class']
  }>(),
  { rows: 5, itemClass: 'h-12 w-full rounded-md', layout: 'stack', class: undefined },
)
</script>

<template>
  <div
    :class="
      cn(
        props.layout === 'grid' ? 'grid gap-4 sm:grid-cols-2 lg:grid-cols-3' : 'space-y-2',
        props.class,
      )
    "
    aria-busy="true"
  >
    <Skeleton v-for="i in props.rows" :key="i" :class="props.itemClass" />
  </div>
</template>
