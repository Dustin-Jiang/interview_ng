<!--
  EmptyState —— 列表/聊天区统一空态：图标 + 主文案 + 次要提示 + 动作插槽。
  收编原先 6 处手写实现（gap-2/gap-3、py-12/py-14 曾各自漂移），此处统一为
  `gap-2 py-14`；bare=true 用于非卡片容器（如聊天滚动区）。
-->
<script setup lang="ts">
import type { Component, HTMLAttributes } from 'vue'

import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    /** 空态图标（lucide 组件），统一样本尺寸与弱化透明度。 */
    icon?: Component
    /** 不包裹 Card，直接输出内容栈（用于已处于其他容器的场景）。 */
    bare?: boolean
    class?: HTMLAttributes['class']
  }>(),
  { icon: undefined, bare: false, class: undefined },
)

const stackClass = 'flex flex-col items-center justify-center gap-2 py-14 text-center text-muted-foreground'
</script>

<template>
  <Card v-if="!props.bare">
    <CardContent :class="cn(stackClass, props.class)">
      <component :is="props.icon" v-if="props.icon" class="h-8 w-8 opacity-50" aria-hidden="true" />
      <p><slot /></p>
      <!-- 次要提示行（引导文案）。 -->
      <p v-if="$slots.hint" class="text-xs"><slot name="hint" /></p>
      <!-- 动作区（如「新建第一个房间」）。 -->
      <div v-if="$slots.action"><slot name="action" /></div>
    </CardContent>
  </Card>
  <div v-else :class="cn(stackClass, props.class)">
    <component :is="props.icon" v-if="props.icon" class="h-8 w-8 opacity-50" aria-hidden="true" />
    <p><slot /></p>
    <p v-if="$slots.hint" class="text-xs"><slot name="hint" /></p>
    <div v-if="$slots.action"><slot name="action" /></div>
  </div>
</template>
