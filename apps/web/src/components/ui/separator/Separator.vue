<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { SeparatorProps } from 'reka-ui'
import { Separator } from 'reka-ui'

import { cn } from '@/lib/utils'

/**
 * Separator —— 语义化分隔线（reka-ui Separator）。
 * 默认横向；orientation="vertical" 时为纵向满高线。
 */
const props = defineProps<SeparatorProps & { class?: HTMLAttributes['class'] }>()
</script>

<template>
  <Separator
    v-bind="props"
    :class="
      cn(
        'shrink-0 bg-border',
        props.orientation === 'vertical' ? 'h-full w-px' : 'h-px w-full',
        props.class,
      )
    "
  />
</template>
