<!--
  ConfirmDialog —— 应用级确认对话框（替代 window.confirm / window.prompt）。
  用于删除等破坏性操作的二次确认：明确后果描述 + destructive 视觉意图 + 提交 loading 态。
-->
<script setup lang="ts">
import { Info, TriangleAlert } from 'lucide-vue-next'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { IconBadge } from '@/components/ui/icon-badge'
import { Spinner } from '@/components/ui/spinner'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    description?: string
    confirmText?: string
    cancelText?: string
    /** 破坏性操作：确认按钮与图标使用 destructive 色。 */
    destructive?: boolean
    /** 确认动作进行中：禁用按钮并阻止关闭。 */
    loading?: boolean
  }>(),
  {
    description: '',
    confirmText: '确认',
    cancelText: '取消',
    destructive: false,
    loading: false,
  },
)

const emit = defineEmits<{
  'update:open': [value: boolean]
  confirm: []
}>()

/** loading 中禁止通过遮罩/Esc 关闭，避免动作悬空。 */
function onOpenChange(value: boolean) {
  if (!value && props.loading) return
  emit('update:open', value)
}

function onConfirm() {
  if (!props.loading) emit('confirm')
}
</script>

<template>
    <Dialog :open="open" @update:open="onOpenChange">
    <DialogContent size="sm">
      <DialogHeader>
        <div class="mb-1 flex items-center gap-3">
          <IconBadge size="md" :tone="destructive ? 'destructive' : 'primary'">
            <TriangleAlert v-if="destructive" aria-hidden="true" />
            <Info v-else aria-hidden="true" />
          </IconBadge>
          <DialogTitle class="text-left">{{ title }}</DialogTitle>
        </div>
        <DialogDescription v-if="description" class="text-left">
          {{ description }}
        </DialogDescription>
      </DialogHeader>
      <!-- 具名插槽：承载额外表单内容（如重置密码输入框）。 -->
      <slot />
      <DialogFooter>
        <Button variant="outline" :disabled="loading" @click="emit('update:open', false)">
          {{ cancelText }}
        </Button>
        <Button :variant="destructive ? 'destructive' : 'default'" :disabled="loading" @click="onConfirm">
          <Spinner v-if="loading" />
          {{ confirmText }}
        </Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
