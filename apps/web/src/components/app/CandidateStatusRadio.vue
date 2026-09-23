<!--
  CandidateStatusRadio —— 候选人状态单选组（七档状态的中文标签，取自 STATUS_PRESENTATION）。
  与 CandidateStatusSelect 契约一致（allowAll / allLabel / ariaLabel），供筛选浮层这类「选项一览、
  一点即选」的场景替代下拉：选项少且互相排斥时，单选组比多两次点击的下拉更快。
  原生 radio：同 name 互斥、方向键在组内移动、单 Tab 停靠点。
-->
<script setup lang="ts">
import { computed, useId } from 'vue'

import { CANDIDATE_STATUSES, type CandidateStatus } from '@/models'
import { STATUS_PRESENTATION } from '@/presenters/status'

const props = withDefaults(
  defineProps<{
    /** 是否提供「全部状态」项（选中即空串）。 */
    allowAll?: boolean
    /** 「全部状态」项的文案。 */
    allLabel?: string
    /** 单选组的无障碍标签。 */
    ariaLabel?: string
  }>(),
  { allowAll: false, allLabel: '全部状态', ariaLabel: '按状态筛选' },
)

const model = defineModel<CandidateStatus | ''>({ required: true })

/** 单选组名：同一表单内多组 radio 靠 name 隔离。 */
const groupName = useId()

interface Option {
  value: CandidateStatus | ''
  label: string
}

const options = computed<Option[]>(() => [
  ...(props.allowAll ? [{ value: '' as const, label: props.allLabel }] : []),
  ...CANDIDATE_STATUSES.map((s) => ({ value: s, label: STATUS_PRESENTATION[s].label })),
])
</script>

<template>
  <div role="radiogroup" :aria-label="props.ariaLabel" class="grid grid-cols-2 gap-1.5">
    <!-- 每行整块可点，触屏下高度 ≥44px；选中项除圆点外再用底色/描边区分（不依赖单一颜色通道）。 -->
    <label
      v-for="opt in options"
      :key="opt.value || 'ALL'"
      class="flex cursor-pointer items-center gap-2 rounded-md border px-2 text-sm transition-colors hover:bg-accent/50 has-[:checked]:border-primary has-[:checked]:bg-accent/40 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring max-lg:min-h-11 lg:min-h-9"
    >
      <input
        type="radio"
        class="size-4 accent-primary"
        :name="groupName"
        :value="opt.value"
        :checked="model === opt.value"
        @change="model = opt.value"
      />
      <span class="min-w-0 truncate">{{ opt.label }}</span>
    </label>
  </div>
</template>
