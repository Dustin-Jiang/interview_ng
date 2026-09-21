<!--
  SettingsImportsView —— 管理员数据导入（设置页分区，需 candidates.manage）。
  三步：① 选择工作簿 → ② 字段映射 + 实时预览 → ③ 确认与提交；
  步骤条只作进度指示（不可点击跳步），流程状态由 useCandidateImport 持有，可回退且保留输入。
-->
<script setup lang="ts">
import ImportFileStep from '@/components/app/ImportFileStep.vue'
import ImportMappingStep from '@/components/app/ImportMappingStep.vue'
import ImportSubmitStep from '@/components/app/ImportSubmitStep.vue'
import PageShell from '@/components/app/PageShell.vue'
import { Button } from '@/components/ui/button'
import { useCandidateImport } from '@/composables/useCandidateImport'
import { cn } from '@/lib/utils'

const {
  step,
  sheet,
  parsing,
  parseError,
  mapping,
  outcome,
  existingLoading,
  existingError,
  presets,
  submitting,
  report,
  rowErrors,
  canSubmit,
  pickFile,
  clearFile,
  setExpression,
  goStep,
  reloadExisting,
  savePreset,
  applyPreset,
  removePreset,
  submit,
  reset,
} = useCandidateImport()

const STEPS = [
  { step: 1, title: '选择文件' },
  { step: 2, title: '字段映射' },
  { step: 3, title: '确认导入' },
]

/** 步骤指示的圆点样式：当前档实底主色、已越过档弱实底、未到档最弱。 */
function stepDotClass(target: number): string {
  if (target === step.value) return 'bg-primary text-primary-foreground'
  if (target < step.value) return 'bg-accent text-accent-foreground'
  return 'text-muted-foreground/50'
}
</script>

<template>
  <PageShell title="数据导入">
    <!-- 步骤指示：只读进度（不提供可点击的跳步入口），回退一律走页内按钮。 -->
    <ol class="flex items-center gap-3" aria-label="导入步骤">
      <li
        v-for="item in STEPS"
        :key="item.step"
        class="flex w-full items-center gap-2"
        :aria-current="item.step === step ? 'step' : undefined"
      >
        <span
          :class="cn('inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm', stepDotClass(item.step))"
        >
          {{ item.step }}
        </span>
        <span :class="cn('whitespace-nowrap text-sm font-semibold', item.step === step ? '' : 'text-muted-foreground')">
          {{ item.title }}
        </span>
        <span v-if="item.step < STEPS.length" class="h-px flex-1 bg-border" />
      </li>
    </ol>

    <ImportFileStep
      v-if="step === 1"
      :sheet="sheet"
      :parsing="parsing"
      :error="parseError"
      @pick="pickFile"
      @clear="clearFile"
      @next="goStep(2)"
    />

    <template v-else-if="step === 2 && sheet">
      <ImportMappingStep
        :sheet="sheet"
        :mapping="mapping"
        :outcome="outcome"
        :presets="presets"
        :existing-loading="existingLoading"
        :existing-error="existingError"
        @expression="setExpression"
        @apply-preset="applyPreset"
        @save-preset="savePreset"
        @remove-preset="removePreset"
        @retry-existing="reloadExisting"
      />
      <div class="flex items-center gap-2">
        <Button variant="outline" size="sm" @click="goStep(1)">上一步</Button>
        <Button size="sm" :disabled="!canSubmit" @click="goStep(3)">下一步</Button>
      </div>
    </template>

    <template v-else>
      <ImportSubmitStep
        :mapped="outcome.mapped"
        :submitting="submitting"
        :report="report"
        :row-errors="rowErrors"
        @submit="submit"
        @reset="reset"
        @back="goStep(2)"
      />
    </template>
  </PageShell>
</template>
