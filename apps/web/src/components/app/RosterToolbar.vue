<!--
  RosterToolbar —— 名册栏顶部工具条：标题（图标 + 文字）+ 元信息 + 右侧动作。
  收编「候选人查看」与「捡漏竞拍」两侧逐字相同的工具条：
  标题走 props，元信息（结果数 / 阶段徽章）走 #meta，右侧按钮走 #actions。
-->
<script setup lang="ts">
import type { Component } from 'vue'

const props = defineProps<{
  title: string
  icon: Component
}>()
</script>

<template>
  <div class="flex items-center gap-2 p-3">
    <h1 class="flex min-w-0 items-center gap-2 truncate text-base font-semibold tracking-tight">
      <component :is="props.icon" class="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      {{ props.title }}
    </h1>
    <slot name="meta" />
    <span class="ml-auto flex shrink-0 items-center gap-1">
      <slot name="actions" />
    </span>
  </div>
</template>
