<!--
  FormDialog —— 增改表单对话框（新增/编辑共用）。
  统一「标题 + 表单字段 + 取消/提交」骨架与提交 loading 态；字段由默认插槽提供，
  可选 #trigger 插槽用于在页头动作区渲染打开按钮。
-->
<script setup lang="ts">
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

const props = withDefaults(
  defineProps<{
    open: boolean
    /** 标题（按模式传入，如「新增部门」/「编辑部门」）。 */
    title: string
    submitText?: string
    /** 提交进行中：禁用两个按钮并显示 spinner。 */
    loading?: boolean
    /** 业务前置未满足（如必填为空）：额外禁用提交按钮。 */
    submitDisabled?: boolean
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { submitText: '保存', loading: false, submitDisabled: false, size: 'md' },
)

const emit = defineEmits<{
  'update:open': [value: boolean]
  submit: []
}>()

/** 提交中禁止关闭，避免动作悬空。 */
function onOpenChange(value: boolean) {
  if (!value && props.loading) return
  emit('update:open', value)
}
</script>

<template>
  <Dialog :open="props.open" @update:open="onOpenChange">
    <DialogTrigger v-if="$slots.trigger" as-child>
      <slot name="trigger" />
    </DialogTrigger>
    <DialogContent :size="props.size">
      <DialogHeader>
        <DialogTitle>{{ props.title }}</DialogTitle>
      </DialogHeader>
      <div class="grid gap-4">
        <slot />
      </div>
      <DialogFooter>
        <Button variant="outline" :disabled="props.loading" @click="emit('update:open', false)">
          取消
        </Button>
        <Button :disabled="props.loading || props.submitDisabled" @click="emit('submit')">
          <Spinner v-if="props.loading" />
          {{ props.submitText }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
