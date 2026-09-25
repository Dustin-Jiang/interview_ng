<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { type KbdVariants, kbdVariants } from '.'
import { cn } from '@/lib/utils'

interface Props {
  tone?: KbdVariants['tone']
  class?: HTMLAttributes['class']
}
const props = defineProps<Props>()
</script>

<template>
  <!-- 用 `<kbd>` 元素：语义上就是「用户从键盘输入的文本」 -->
  <kbd :class="cn(kbdVariants({ tone: props.tone }), props.class)">
    <slot />
  </kbd>
</template>
