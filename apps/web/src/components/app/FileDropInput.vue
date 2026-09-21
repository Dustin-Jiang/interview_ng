<!--
  FileDropInput —— 文件选择（原生 input + 拖拽投放）。
  收编「点击选择 / 拖入文件」两种交互：拖拽态用 bg-accent 与边框高亮反馈，
  选中后由调用方通过标签插槽展示文件名等结果。
-->
<script setup lang="ts">
import { ref, type HTMLAttributes } from 'vue'
import { Upload } from 'lucide-vue-next'

import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    /** input 的 accept 属性（默认只收 .xlsx）。 */
    accept?: string
    /** 是否禁用（解析进行中）。 */
    disabled?: boolean
    class?: HTMLAttributes['class']
  }>(),
  { accept: '.xlsx', disabled: false, class: undefined },
)

const emit = defineEmits<{ pick: [file: File] }>()

const input = ref<HTMLInputElement | null>(null)
const dragging = ref(false)

function submit(files: FileList | null): void {
  if (props.disabled) return
  const file = files?.[0]
  if (file) emit('pick', file)
}

function onDrop(event: DragEvent): void {
  dragging.value = false
  submit(event.dataTransfer?.files ?? null)
}

/** 选择同一文件也要能再次触发 change，故每次选完清空 input 值。 */
function onPicked(event: Event): void {
  const el = event.target as HTMLInputElement
  submit(el.files)
  el.value = ''
}
</script>

<template>
  <div
    :class="
      cn(
        'flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed p-8 text-center transition-colors',
        dragging ? 'border-ring bg-accent/50' : 'bg-card',
        props.disabled ? 'opacity-60' : 'cursor-pointer hover:bg-accent/40',
        props.class,
      )
    "
    role="button"
    tabindex="0"
    :aria-disabled="props.disabled"
    @click="input?.click()"
    @keydown.enter.prevent="input?.click()"
    @keydown.space.prevent="input?.click()"
    @dragover.prevent="dragging = true"
    @dragleave.prevent="dragging = false"
    @drop.prevent="onDrop"
  >
    <Upload class="h-6 w-6 text-muted-foreground" aria-hidden="true" />
    <p class="text-sm font-medium">{{ dragging ? '松开即选中文件' : '点击选择或拖入文件' }}</p>
    <slot />
    <input
      ref="input"
      type="file"
      class="hidden"
      :accept="props.accept"
      :disabled="props.disabled"
      @change="onPicked"
    />
  </div>
</template>
