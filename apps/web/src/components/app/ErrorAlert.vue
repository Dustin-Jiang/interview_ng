<!--
  ErrorAlert —— 拉取失败提示（role=alert + 可选重试）。
  variant="panel"：卡片/滚动区内的带底色提示块；variant="plain"：列表容器内的纯文字提示。
-->
<script setup lang="ts">
import type { HTMLAttributes } from 'vue'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    /** 提示文案（调用方按需补充前缀，如「预算信息加载失败：」）。 */
    message: string
    /** 有值时展示重试按钮。 */
    retryLabel?: string
    variant?: 'panel' | 'plain'
    class?: HTMLAttributes['class']
  }>(),
  { retryLabel: '', variant: 'panel', class: undefined },
)

defineEmits<{ retry: [] }>()
</script>

<template>
  <div
    role="alert"
    :class="
      cn(
        'text-sm text-destructive',
        props.variant === 'panel' ? 'rounded-md bg-destructive/10 px-3 py-2' : 'p-4',
        props.class,
      )
    "
  >
    {{ props.message }}
    <Button v-if="props.retryLabel" variant="link" class="h-auto p-0 max-lg:h-11 max-lg:px-2" @click="$emit('retry')">
      {{ props.retryLabel }}
    </Button>
  </div>
</template>
