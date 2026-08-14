<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { SelectContentEmits, SelectContentProps } from 'reka-ui'
import { SelectContent, SelectViewport, useForwardPropsEmits } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<SelectContentProps & { class?: HTMLAttributes['class'] }>()
const emits = defineEmits<SelectContentEmits>()
const forwarded = useForwardPropsEmits(props, emits)
</script>

<template>
  <SelectContent v-bind="forwarded" :class="cn('relative z-50 max-h-96 min-w-[8rem] overflow-hidden rounded-md border bg-popover text-popover-foreground shadow-md', props.class)">
    <SelectViewport class="p-1">
      <slot />
    </SelectViewport>
  </SelectContent>
</template>
