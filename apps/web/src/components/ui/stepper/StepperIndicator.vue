<script setup lang="ts">
import type { StepperIndicatorProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { StepperIndicator, useForwardProps } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<StepperIndicatorProps & { class?: HTMLAttributes['class'] }>()

const delegated = computed(() => {
  const { class: _class, ...rest } = props
  return rest
})
const forwarded = useForwardProps(delegated)
</script>

<template>
  <StepperIndicator
    v-slot="slotProps"
    v-bind="forwarded"
    :class="cn(
      'inline-flex h-8 w-8 items-center justify-center rounded-full text-muted-foreground/50',
      // 禁用态
      'group-data-[disabled]:text-muted-foreground group-data-[disabled]:opacity-50',
      // 当前档
      'group-data-[state=active]:bg-primary group-data-[state=active]:text-primary-foreground',
      // 已越过档
      'group-data-[state=completed]:bg-accent group-data-[state=completed]:text-accent-foreground',
      props.class,
    )"
  >
    <slot v-bind="slotProps" />
  </StepperIndicator>
</template>
