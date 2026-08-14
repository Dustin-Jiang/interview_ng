<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import { cn } from '@/lib/utils'

interface Props {
  /** v-model：输入值 */
  modelValue?: string | number
  type?: 'text' | 'password' | 'email' | 'number' | 'search' | 'tel' | 'url'
  placeholder?: string
  disabled?: boolean
  readonly?: boolean
  inputmode?: 'text' | 'numeric' | 'decimal' | 'email' | 'search' | 'tel' | 'url' | 'none'
  class?: HTMLAttributes['class']
  maxlength?: number
  autocomplete?: string
}
const props = withDefaults(defineProps<Props>(), { type: 'text' })

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
  change: [event: Event]
}>()

function onInput(event: Event) {
  const target = event.target as HTMLInputElement
  emit('update:modelValue', target.value)
}
</script>

<template>
  <input
    :type="props.type"
    :value="props.modelValue"
    :placeholder="props.placeholder"
    :inputmode="props.inputmode"
    :disabled="props.disabled"
    :readonly="props.readonly"
    :maxlength="props.maxlength"
    :autocomplete="props.autocomplete"
    :class="
      cn(
        'flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
        props.class,
      )
    "
    @input="onInput"
    @change="emit('change', $event)"
  />
</template>
