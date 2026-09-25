<!--
  MessageEditor —— 消息的就地编辑框（气泡位置换成多行输入 + 取消 / 保存）。
  Enter 保存、Shift+Enter 换行、Esc 取消；打开即聚焦（调用方用 `open` 触发）。
  纯受控组件：草稿与保存状态由持有方管理（服务端权威）。

  样式三条约定（都是「就地编辑」这件事的需要，不要随手改回去）：
  1. **几何与气泡一致**（`rounded-2xl` + `px-3.5 py-2` + `text-sm leading-relaxed`，
     同 `ui/tokens.ts#chatBubbleVariants`）：气泡与编辑框在同一块地方互相替换，
     圆角/内距/行高一致才不跳版；`ui/textarea` 自带的 `rounded-md px-3 py-1.5` 是表单语言，
     直接放在气泡位置上会显得是「另一个控件」。
  2. **宽度也跟气泡一致**（`max-w-[85%] sm:max-w-[75%]`，与气泡同一口径）：编辑长消息时
     不再比气泡更宽/更窄（此前固定 `max-w-lg`，900px 视口下气泡 651px 而编辑框只有 512px，
     一进编辑整行就缩）。手机（`max-sm`，<640）例外：放宽到全宽，打字留够空间
     —— 用 `max-sm` 而不是 `max-lg`：`max-lg` 与 `sm:max-w-[75%]` 在 640~1023 同时命中，
     谁生效取决于样式表顺序，写 `max-sm` 才是「只有手机全宽」这个确定语义。
  3. **保存中显示 spinner**（同 `MessageComposer` 的发送态）：只禁用按钮不足以说明「正在写」。
  焦点环沿用 `ui/input`/`ui/textarea` 的 `ring-1`，不另立一套。
-->
<script setup lang="ts">
import { nextTick, ref, watch, type ComponentPublicInstance } from 'vue'

import { Button } from '@/components/ui/button'
import { Kbd } from '@/components/ui/kbd'
import { Spinner } from '@/components/ui/spinner'
import { Textarea } from '@/components/ui/textarea'

const props = defineProps<{
  /** 输入内容（v-model）。 */
  modelValue: string
  /** 保存中：禁用按钮、阻止重复提交。 */
  saving?: boolean
  /** 打开标记：由 false 变 true 时聚焦。 */
  open: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  save: []
  cancel: []
}>()

/** 编辑框元素：函数 ref（v-for 内的模板 ref 会收成数组）。 */
let editorEl: HTMLTextAreaElement | null = null
function captureEditor(el: Element | ComponentPublicInstance | null): void {
  const root = (el as { $el?: unknown } | null)?.$el
  editorEl = ((root ?? el) as HTMLTextAreaElement | null) ?? null
}

watch(
  () => props.open,
  (open) => {
    if (open) void nextTick(() => editorEl?.focus())
  },
  { immediate: true },
)
</script>

<template>
  <div class="flex w-full min-w-0 max-w-[85%] flex-col gap-2 sm:max-w-[75%] max-sm:max-w-full">
    <!-- resize-y：默认的 resize: both 能横向拖窄编辑框、把整行拽变形，纵向留作「写长了拉高」 -->
    <Textarea
      :ref="captureEditor"
      :model-value="props.modelValue"
      class="resize-y rounded-2xl px-3.5 py-2 leading-relaxed max-lg:min-h-24"
      aria-label="编辑面试记录"
      @update:model-value="emit('update:modelValue', $event)"
      @keydown.enter.exact.prevent="emit('save')"
      @keydown.esc="emit('cancel')"
    />
    <div class="flex items-center justify-end gap-2">
      <Button variant="ghost" size="sm" :disabled="props.saving" @click="emit('cancel')">
        取消
        <Kbd>Esc</Kbd>
      </Button>
      <Button size="sm" :disabled="props.saving || !props.modelValue.trim()" @click="emit('save')">
        <Spinner v-if="props.saving" />
        保存
        <Kbd>Enter</Kbd>
      </Button>
    </div>
  </div>
</template>
