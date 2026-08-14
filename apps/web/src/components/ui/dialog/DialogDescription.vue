<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { DialogDescription } from 'reka-ui'
import { cn } from '@/lib/utils'

interface Props {
  class?: HTMLAttributes['class']
  asChild?: boolean
}
const props = defineProps<Props>()
</script>

<template>
  <DialogDescription
    :as-child="props.asChild"
    :class="cn('text-sm text-muted-foreground', props.class)"
  >
    <slot />
  </DialogDescription>
</template>
