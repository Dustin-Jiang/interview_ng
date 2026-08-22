<script setup lang="ts">
import type { DialogTitleProps } from 'reka-ui'
import type { HTMLAttributes } from 'vue'
import { DialogTitle, useForwardProps } from 'reka-ui'
import { cn } from '@/lib/utils'

const props = defineProps<DialogTitleProps & { class?: HTMLAttributes['class'] }>()

const { class: _class, ...delegated } = props
const forwarded = useForwardProps(delegated)
</script>

<template>
  <DialogTitle
    v-bind="forwarded"
    :class="cn('text-lg font-semibold leading-none tracking-tight', props.class)"
  >
    <slot />
  </DialogTitle>
</template>
