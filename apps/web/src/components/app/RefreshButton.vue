<!--
  RefreshButton —— 列表页统一刷新按钮。
  收编各视图重复的「图标按钮 + loading 时 RefreshCw 旋转」实现，加载中禁用避免并发重入。
  variant="ghost" 用于紧凑工具条（配合 class="h-8 w-8"），默认 outline 用于页头动作区。
-->
<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { RefreshCw } from 'lucide-vue-next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    /** 加载中：图标旋转并禁用按钮。 */
    loading?: boolean
    /** 无障碍标签，须自明（如「刷新房间列表」）。 */
    label: string
    variant?: 'outline' | 'ghost'
    class?: HTMLAttributes['class']
  }>(),
  { loading: false, variant: 'outline', class: undefined },
)

defineEmits<{ click: [] }>()
</script>

<template>
  <Button
    :variant="props.variant"
    size="icon"
    :class="cn(props.class)"
    :disabled="props.loading"
    :aria-label="props.label"
    @click="$emit('click')"
  >
    <RefreshCw :class="props.loading ? 'animate-spin' : ''" aria-hidden="true" />
  </Button>
</template>
