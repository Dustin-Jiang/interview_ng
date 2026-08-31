<script setup lang="ts">
import type { StepperSeparatorProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { StepperSeparator, useForwardProps } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<StepperSeparatorProps & { class?: HTMLAttributes['class'] }>()

const delegated = computed(() => {
  const { class: _class, ...rest } = props
  return rest
})
const forwarded = useForwardProps(delegated)
</script>

<template>
  <StepperSeparator
    v-bind="forwarded"
    :class="cn(
      'bg-muted',
      // 禁用态
      'group-data-[disabled]:bg-muted group-data-[disabled]:opacity-50',
      // 已越过档
      'group-data-[state=completed]:bg-accent',
      props.class,
    )"
  />
</template>
