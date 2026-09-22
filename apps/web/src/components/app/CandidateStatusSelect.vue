<!--
  CandidateStatusSelect —— 候选人状态下拉（七档状态的中文标签，取自 STATUS_PRESENTATION）。
  收编「候选人查看 / 候选人管理」两处逐字相同的筛选下拉，以及「重置状态」的目标档选择
  （该处不带「全部」项，value 恒为具体状态）。
-->
<script setup lang="ts">
import { CANDIDATE_STATUSES, type CandidateStatus } from '@/models'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const props = withDefaults(
  defineProps<{
    /** 是否提供「全部状态」项（选中即空串）。 */
    allowAll?: boolean
    /** 「全部状态」项的文案与占位符。 */
    allLabel?: string
    triggerClass?: string
    /** 触发按钮的无障碍标签。 */
    ariaLabel?: string
    /** 触发按钮 id（供 Label 关联）。 */
    triggerId?: string
  }>(),
  {
    allowAll: false,
    allLabel: '全部状态',
    triggerClass: 'w-full',
    ariaLabel: '按状态筛选',
    triggerId: undefined,
  },
)

const model = defineModel<CandidateStatus | ''>({ required: true })
</script>

<template>
  <Select
    :model-value="model || 'ALL'"
    @update:model-value="model = $event === 'ALL' ? '' : ($event as CandidateStatus)"
  >
    <SelectTrigger :id="props.triggerId" :class="props.triggerClass" :aria-label="props.ariaLabel">
      <SelectValue :placeholder="props.allLabel" />
    </SelectTrigger>
    <SelectContent>
      <SelectItem v-if="props.allowAll" value="ALL">{{ props.allLabel }}</SelectItem>
      <SelectItem v-for="s in CANDIDATE_STATUSES" :key="s" :value="s">
        {{ STATUS_PRESENTATION[s].label }}
      </SelectItem>
    </SelectContent>
  </Select>
</template>
