<!--
  MessageComposer —— 面试记录的输入区（房间聊天与候选人查看页的补充记录共用一份）。
  受控：草稿由持有方管理；Enter 发送、发送中显示 spinner 并禁用（防重复提交）。
  `disabled` 时输入框与发送键同步禁用——不可用原因写在 placeholder 里（交给调用方决定文案）。
  待发引用（`quoted`）在输入框上方显示「正在引用」条，由持有方在发送成功或引用目标撤回后清空。
-->
<script setup lang="ts">
import { Send, X } from 'lucide-vue-next'

import { cn } from '@/lib/utils'
import type { Message } from '@/models'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { quotedBlockVariants } from '@/components/ui/tokens'
import QuotedMessageRef from '@/components/app/QuotedMessageRef.vue'

const props = withDefaults(
  defineProps<{
    /** 草稿（v-model）。 */
    modelValue: string
    /** 不可发送（无权限 / 面试已结档 / 房间空闲等）——原因由调用方写进 placeholder。 */
    disabled?: boolean
    /** 发送中：输入框保留内容但禁用，按钮显示 spinner。 */
    sending?: boolean
    placeholder?: string
    /** 输入框的无障碍标签。 */
    label?: string
    /** 待发引用目标：非空时输入框上方显示「正在引用」条。 */
    quoted?: Message | null
  }>(),
  {
    disabled: false,
    sending: false,
    placeholder: '输入面试记录，Enter 发送…',
    label: '消息内容',
    quoted: undefined,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: string]
  send: []
  /** 取消待发引用（清空持有方的 quoteTarget）。 */
  'cancel-quote': []
}>()

const canSubmit = () => !props.disabled && !props.sending && props.modelValue.trim().length > 0
</script>

<template>
  <div class="flex min-w-0 flex-col">
    <!-- 「正在引用」条：待发引用目标（被引用消息撤回时由持有方的 watch 提前清空，故这里不显示占位） -->
    <div
      v-if="props.quoted"
      :class="cn(quotedBlockVariants(), 'mb-2 flex items-center gap-2 py-1.5')"
    >
      <QuotedMessageRef :message="props.quoted" class="flex-1" />
      <button
        type="button"
        class="shrink-0 rounded-sm p-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-lg:h-11 max-lg:w-11"
        aria-label="取消引用"
        @click="emit('cancel-quote')"
      >
        <X class="h-4 w-4" aria-hidden="true" />
      </button>
    </div>
    <div class="flex min-w-0 items-center gap-2">
      <Input
        :model-value="props.modelValue"
        class="min-w-0"
        :disabled="props.disabled || props.sending"
        :placeholder="props.placeholder"
        :aria-label="props.label"
        @update:model-value="emit('update:modelValue', String($event))"
        @keydown.enter.prevent="canSubmit() && emit('send')"
      />
      <Button
        class="shrink-0"
        size="icon"
        :aria-label="props.sending ? '发送中' : '发送消息'"
        :disabled="!canSubmit()"
        @click="emit('send')"
      >
        <Spinner v-if="props.sending" />
        <Send v-else aria-hidden="true" />
      </Button>
    </div>
  </div>
</template>
