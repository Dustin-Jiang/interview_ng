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
    <!-- 标题在 flex 行内：文字必须包一层 truncate 节点，否则溢出时只会被裁掉而不出省略号 -->
    <h1 class="flex min-w-0 items-center gap-2 text-base font-semibold tracking-tight">
      <component :is="props.icon" class="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
      <span class="truncate">{{ props.title }}</span>
    </h1>
    <slot name="meta" />
    <!-- 右侧动作：恒不收缩（标题先让位），手机上两个图标按钮各 44px 仍可容纳 -->
    <span class="ml-auto flex shrink-0 items-center gap-1">
      <slot name="actions" />
    </span>
  </div>
</template>
