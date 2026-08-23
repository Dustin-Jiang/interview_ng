<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { AvatarImageProps } from 'reka-ui'
import { AvatarImage } from 'reka-ui'

import { cn } from '@/lib/utils'

/** Avatar —— 头像图片（加载失败自动回落到 Fallback）。 */
const props = defineProps<AvatarImageProps & { class?: HTMLAttributes['class'] }>()
</script>

<template>
  <AvatarImage v-bind="props" :class="cn('aspect-square h-full w-full object-cover', props.class)" />
</template>
