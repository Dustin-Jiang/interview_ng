<script setup lang="ts">
import type { StepperDescriptionProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { StepperDescription, useForwardProps } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<StepperDescriptionProps & { class?: HTMLAttributes['class'] }>()

const delegated = computed(() => {
  const { class: _class, ...rest } = props
  return rest
})
const forwarded = useForwardProps(delegated)
</script>

<template>
  <StepperDescription
    v-slot="slotProps"
    v-bind="forwarded"
    :class="cn('text-xs text-muted-foreground', props.class)"
  >
    <slot v-bind="slotProps" />
  </StepperDescription>
</template>
