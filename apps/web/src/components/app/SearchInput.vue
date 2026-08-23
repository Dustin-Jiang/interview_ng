<!--
  SearchInput —— 带图标与一键清空的搜索输入框。
  收编原先 CandidatesView / UsersView 各自重复约 20 行的逐字实现。
  Enter 或清空均触发 search（携带当前关键词），由调用方决定如何检索。
-->
<script setup lang="ts">
import { Search, X } from 'lucide-vue-next'
import type { HTMLAttributes } from 'vue'

import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    placeholder?: string
    /** 撑满父容器宽度（默认固定 w-56，用于表格工具栏）。 */
    full?: boolean
    class?: HTMLAttributes['class']
  }>(),
  { full: false },
)

const emit = defineEmits<{ search: [value: string] }>()

/** v-model：搜索关键词。 */
const keyword = defineModel<string>({ default: '' })

function onClear() {
  keyword.value = ''
  emit('search', '')
}
</script>

<template>
  <div :class="cn('relative', props.class)">
    <Search
      class="pointer-events-none absolute left-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground"
      aria-hidden="true"
    />
    <Input
      v-model="keyword"
      type="text"
      :placeholder="placeholder"
      :class="props.full ? 'w-full pl-8 pr-8' : 'w-56 pl-8 pr-8'"
      @keydown.enter="emit('search', String(keyword))"
    />
    <button
      v-if="keyword"
      type="button"
      class="absolute right-2 top-1/2 -translate-y-1/2 rounded-sm p-0.5 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      aria-label="清空搜索"
      @click="onClear"
    >
      <X class="h-4 w-4" aria-hidden="true" />
    </button>
  </div>
</template>
