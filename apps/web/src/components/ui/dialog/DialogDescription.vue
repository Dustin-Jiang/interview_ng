<script setup lang="ts">
import type { DialogDescriptionProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { DialogDescription, useForwardProps } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<DialogDescriptionProps & { class?: HTMLAttributes['class'] }>()

const { class: _class, ...delegated } = props
const forwarded = useForwardProps(delegated)
</script>

<template>
  <DialogDescription
    v-bind="forwarded"
    :class="cn('text-sm text-muted-foreground', props.class)"
  >
    <slot />
  </DialogDescription>
</template>
