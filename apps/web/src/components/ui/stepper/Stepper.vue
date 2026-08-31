<script setup lang="ts">
import type { StepperRootEmits, StepperRootProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { computed } from 'vue'
import { StepperRoot, useForwardPropsEmits } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<StepperRootProps & { class?: HTMLAttributes['class'] }>()
const emits = defineEmits<StepperRootEmits>()

// 响应式剥离 class（快照会冻结 modelValue 等动态 prop 的变更）。
const delegated = computed(() => {
  const { class: _class, ...rest } = props
  return rest
})
const forwarded = useForwardPropsEmits(delegated, emits)
</script>

<template>
  <StepperRoot
    v-slot="slotProps"
    v-bind="forwarded"
    :class="cn('flex gap-2', props.class)"
  >
    <slot v-bind="slotProps" />
  </StepperRoot>
</template>
