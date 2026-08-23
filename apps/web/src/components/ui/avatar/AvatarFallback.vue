<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { AvatarFallbackProps } from 'reka-ui'
import { AvatarFallback } from 'reka-ui'

import { cn } from '@/lib/utils'

/** Avatar —— 图片缺省时的回退内容（姓名首字母等）。 */
const props = defineProps<AvatarFallbackProps & { class?: HTMLAttributes['class'] }>()
</script>

<template>
  <AvatarFallback
    v-bind="props"
    :class="cn('flex h-full w-full items-center justify-center rounded-full bg-muted font-medium text-muted-foreground', props.class)"
  >
    <slot />
  </AvatarFallback>
</template>
