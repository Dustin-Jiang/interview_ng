<script setup lang="ts">
import type { NumberFieldDecrementProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { reactiveOmit } from '@vueuse/core'
import { NumberFieldDecrement, useForwardProps } from 'reka-ui'
import { Minus } from 'lucide-vue-next'
import { cn } from '@/lib/utils'

const props = defineProps<NumberFieldDecrementProps & { class?: HTMLAttributes['class'] }>()

const delegatedProps = reactiveOmit(props, 'class')

const forwarded = useForwardProps(delegatedProps)
</script>

<template>
  <NumberFieldDecrement
    data-slot="decrement"
    v-bind="forwarded"
    :class="cn('absolute top-1/2 -translate-y-1/2 left-0 p-2.5 max-lg:p-3.5 disabled:cursor-not-allowed disabled:opacity-20', props.class)"
  >
    <slot>
      <Minus class="h-4 w-4" aria-hidden="true" />
    </slot>
  </NumberFieldDecrement>
</template>
