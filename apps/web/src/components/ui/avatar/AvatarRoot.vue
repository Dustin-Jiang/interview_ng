<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { AvatarRootProps } from 'reka-ui'
import { AvatarRoot } from 'reka-ui'

import { cn } from '@/lib/utils'

/**
 * Avatar —— 头像根组件（reka-ui AvatarRoot）。
 * 尺寸通过 avatarVariants / class 控制，默认 40px 圆形裁剪。
 */
const props = defineProps<AvatarRootProps & { class?: HTMLAttributes['class'] }>()
</script>

<template>
  <AvatarRoot
    v-bind="props"
    :class="cn('relative flex h-10 w-10 shrink-0 overflow-hidden rounded-full', props.class)"
  >
    <slot />
  </AvatarRoot>
</template>
