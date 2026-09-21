<!--
  RosterPager —— 名册顺序切换（上一个 / 下一个，←/→ 键盘可达）。
  由「候选人查看」与「捡漏竞拍」逐字节相同的分页实现收编而来。
-->
<script setup lang="ts">
import { ChevronLeft, ChevronRight } from 'lucide-vue-next'

import { Button } from '@/components/ui/button'

const props = withDefaults(
  defineProps<{
    canPrev: boolean
    canNext: boolean
    label?: string
  }>(),
  { label: '候选人切换' },
)

defineEmits<{ prev: []; next: [] }>()
</script>

<template>
  <nav class="flex items-center justify-between gap-3" :aria-label="props.label">
    <Button variant="outline" class="min-w-0 max-w-[45%] gap-1.5" :disabled="!props.canPrev" @click="$emit('prev')">
      <ChevronLeft class="shrink-0" aria-hidden="true" />
      <span class="min-w-0 truncate">上一个</span>
    </Button>
    <Button variant="outline" class="min-w-0 max-w-[45%] gap-1.5" :disabled="!props.canNext" @click="$emit('next')">
      <span class="min-w-0 truncate">下一个</span>
      <ChevronRight class="shrink-0" aria-hidden="true" />
    </Button>
  </nav>
</template>
