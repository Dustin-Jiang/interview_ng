<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { DialogTitle } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props {
  class?: HTMLAttributes['class']
  asChild?: boolean
}
const props = defineProps<Props>()
</script>

<template>
  <DialogTitle
    :as-child="props.asChild"
    :class="cn('text-lg font-semibold leading-none tracking-tight', props.class)"
  >
    <slot />
  </DialogTitle>
</template>
