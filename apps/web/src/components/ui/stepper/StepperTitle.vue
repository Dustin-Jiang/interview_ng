<script setup lang="ts">
import type { StepperTitleProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { StepperTitle, useForwardProps } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<StepperTitleProps & { class?: HTMLAttributes['class'] }>()

const delegated = computed(() => {
  const { class: _class, ...rest } = props
  return rest
})
const forwarded = useForwardProps(delegated)
</script>

<template>
  <StepperTitle
    v-bind="forwarded"
    :class="cn('text-sm font-semibold whitespace-nowrap', props.class)"
  >
    <slot />
  </StepperTitle>
</template>
