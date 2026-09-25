<!--
  QuotedMessageRef —— 引用块的展示内容（「谁 + 原文」**单行**），**气泡外**的引用块与输入区的
  「正在引用」条共用。只渲染内容，**不自带边框/底色**——表面样式由调用方决定（两处都用
  `ui/tokens.ts#quotedBlockVariants`，宽度另由调用方给：气泡外与气泡同一收缩宽度上限、输入条占满本行）。
  口径统一走 `domain/messages.ts#quotedPreview`：被引用消息已撤回（物理删除）或不在当前记录窗口内时
  查不到，只渲染占位文案。
  定版为**一行**：引用图标 + 发送者 + 原文同排（不显示部门头衔）——引用只是「这条在回哪条」的提示，
  多行会把引用块撑得比气泡还厚重。**发送者完整显示、不省略**（是谁在回必须一眼看清，`shrink-0`
  保住姓名/用户名的完整宽度），只有原文过长才 `truncate` 省略（`flex-1 min-w-0`，先让原文让步）。
-->
<script setup lang="ts">
import { computed } from 'vue'
import { Quote } from 'lucide-vue-next'

import { useAuth } from '@/composables/useAuth'
import { quotedPreview } from '@/domain/messages'
import type { Message } from '@/models'

const props = withDefaults(
  defineProps<{
    /** 被引用的消息（调用方在本地已加载的记录里按 id 现查）；查不到传 null/undefined。 */
    message?: Message | null
    /** 查不到被引用消息时的占位文案。 */
    missingLabel?: string
  }>(),
  { message: null, missingLabel: '引用的消息已撤回' },
)

const { currentUserId } = useAuth()

const preview = computed(() => quotedPreview(props.message ?? undefined, currentUserId.value))
</script>

<template>
  <div class="flex min-w-0 items-center gap-1.5 text-xs leading-snug">
    <Quote class="h-3 w-3 shrink-0 opacity-60" aria-hidden="true" />
    <template v-if="preview">
      <span class="shrink-0 font-medium whitespace-nowrap">{{ preview.label }}</span>
      <span class="min-w-0 flex-1 truncate opacity-80">{{ preview.content }}</span>
    </template>
    <!-- 占位与正文都只降透明度、不取 muted 色：引用块的表面由调用方给（见上），
         继承当前文字色 + 降透明度在两处底色上都有对比，换表面也不会失配。 -->
    <p v-else class="min-w-0 flex-1 truncate opacity-70">{{ props.missingLabel }}</p>
  </div>
</template>
