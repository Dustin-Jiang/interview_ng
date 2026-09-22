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
  submitting,
  report,
  rowErrors,
  canSubmit,
  pickFile,
  clearFile,
  setExpression,
  goStep,
  reloadExisting,
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
    <!-- 步骤指示：只读进度（不提供可点击的跳步入口），回退一律走页内按钮。
         手机上（窄于 md）三档竖排：横排时「圆点 + 三个中文标题 + 连接线」在 360px 下会被 nowrap 标题顶出横向滚动；
         竖排后连接线无意义（隐藏），保留编号圆点与当前档高亮即可表达进度。 -->
    <ol class="flex flex-col gap-2 md:flex-row md:items-center md:gap-3" aria-label="导入步骤">
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
        <span
          :class="
            cn(
              'min-w-0 text-sm font-semibold break-all md:whitespace-nowrap',
              item.step === step ? '' : 'text-muted-foreground',
            )
          "
        >
          {{ item.title }}
        </span>
        <span v-if="item.step < STEPS.length" class="hidden h-px flex-1 bg-border md:block" />
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
        :existing-loading="existingLoading"
        :existing-error="existingError"
        @expression="setExpression"
        @retry-existing="reloadExisting"
      />
      <!-- 手机上按钮已被抬到 h-11，允许换行：文案更长或字号更大时不会把整行顶出容器。 -->
      <div class="flex flex-wrap items-center gap-2">
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
