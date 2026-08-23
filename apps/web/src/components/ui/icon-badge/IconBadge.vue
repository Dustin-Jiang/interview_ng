<script setup lang="ts">
import type { HTMLAttributes } from 'vue'

import { cn } from '@/lib/utils'
import { iconBadgeVariants } from '.'

/**
 * IconBadge —— 圆形图标徽章（头像 / 品牌标识 / 对话框警示图标）。
 * 收编原先 5 处各自手写的「圆形弱化底图标位」实现（尺寸/色调曾各自漂移）。
 * 槽内 svg 图标尺寸随 size 档自动约束。
 * 注：props 用字面量联合声明 —— SFC 编译器无法在 defineProps 内解析
 * cva 的 VariantProps 条件类型（变体定义仍由 index.ts 统一维护）。
 */
const props = withDefaults(
  defineProps<{
    /** 色调：primary 弱化底 / destructive 弱化底 / solid 实底（品牌）。 */
    tone?: 'primary' | 'destructive' | 'solid'
    /** 尺寸档：sm=32px / md=40px / lg=48px。 */
    size?: 'sm' | 'md' | 'lg'
    class?: HTMLAttributes['class']
  }>(),
  { tone: 'primary', size: 'md' },
)
</script>

<template>
  <div :class="cn(iconBadgeVariants({ tone: props.tone, size: props.size }), props.class)">
    <slot />
  </div>
</template>
