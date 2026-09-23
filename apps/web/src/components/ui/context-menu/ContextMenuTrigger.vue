<script setup lang="ts">
import type { ContextMenuTriggerProps } from 'reka-ui'
import { ContextMenuTrigger } from 'reka-ui'

const props = defineProps<ContextMenuTriggerProps>()
</script>

<template>
  <!-- 触发区即右键区域：鼠标右键、触屏长按、键盘菜单键都能打开（reka-ui 内建），
       disabled 时该区域不响应右键（用于「这条记录不可改」的消息）。 -->
  <ContextMenuTrigger v-bind="props">
    <slot />
  </ContextMenuTrigger>
</template>
