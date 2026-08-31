<script setup lang="ts">
import type { StepperTriggerProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { StepperTrigger, useForwardProps } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<StepperTriggerProps & { class?: HTMLAttributes['class'] }>()

const delegated = computed(() => {
  const { class: _class, ...rest } = props
  return rest
})
const forwarded = useForwardProps(delegated)
</script>

<template>
  <StepperTrigger
    v-bind="forwarded"
    :class="cn('flex flex-col items-center gap-1 rounded-md p-1 text-center', props.class)"
  >
    <slot />
  </StepperTrigger>
</template>
