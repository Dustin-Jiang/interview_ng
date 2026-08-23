<script setup lang="ts">
import { LoaderCircle } from 'lucide-vue-next'
import type { HTMLAttributes } from 'vue'

import { cn } from '@/lib/utils'

/**
 * Spinner —— 统一的加载指示（原各视图重复手写 `h-4 w-4 animate-spin`，
 * 且存在 h-4/h-3.5 漂移）。默认尺寸对齐 Button 内置图标（size-4）。
 */
const props = defineProps<{
  class?: HTMLAttributes['class']
}>()
</script>

<template>
  <LoaderCircle :class="cn('h-4 w-4 animate-spin', props.class)" aria-hidden="true" />
</template>
