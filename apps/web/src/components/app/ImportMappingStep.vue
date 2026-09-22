<!--
  ImportMappingStep —— 数据导入第 ② 步：上下布局（字段映射在上、实时预览在下）。
  每个目标字段一条 JMESPath 表达式；中文列名必须加引号，映射卡内给出可直接复制的列名清单。
  表达式输入由调用方防抖（300ms），本组件只负责呈现与收集输入。
-->
<script setup lang="ts">
import { ref } from 'vue'
import { Copy } from 'lucide-vue-next'
import { toast } from 'vue-sonner'

import ErrorAlert from '@/components/app/ErrorAlert.vue'
import ImportPreviewTable from '@/components/app/ImportPreviewTable.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { CANDIDATE_IMPORT_FIELDS, type CandidateImportFieldKey, type CandidateImportMapping, type ImportOutcome, type ImportPreset, type ImportSheet } from '@/domain/import'
import { toastError } from '@/lib/toast'

const props = defineProps<{
  sheet: ImportSheet
  mapping: CandidateImportMapping
  outcome: ImportOutcome
  presets: ImportPreset[]
  existingLoading: boolean
  existingError: string | null
}>()

const emit = defineEmits<{
  expression: [key: CandidateImportFieldKey, value: string]
  applyPreset: [name: string]
  savePreset: [name: string]
  removePreset: [name: string]
  retryExisting: []
}>()

/** 预设选择与命名（保存即覆盖同名预设）。 */
const selectedPreset = ref('')
const presetName = ref('')

function onApplyPreset(name: string): void {
  selectedPreset.value = name
  emit('applyPreset', name)
}

function onSavePreset(): void {
  const name = presetName.value.trim()
  if (!name) {
    toast.error('请输入预设名称')
    return
  }
  emit('savePreset', name)
  selectedPreset.value = name
  presetName.value = ''
  toast.success(`预设「${name}」已保存`)
}

function onRemovePreset(): void {
  if (!selectedPreset.value) return
  emit('removePreset', selectedPreset.value)
  selectedPreset.value = ''
}

/** 复制列名的表达式写法（非 ASCII 起首的标识符必须加引号）。 */
async function copyColumn(header: string): Promise<void> {
  const quoted = `"${header}"`
  try {
    await navigator.clipboard.writeText(quoted)
    toast.success(`已复制 ${quoted}`)
  } catch (e) {
    toastError(e, '复制失败，请手动输入')
  }
}
</script>

<template>
  <div class="space-y-4">
    <Card>
      <CardHeader>
        <CardTitle>字段映射</CardTitle>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="space-y-2">
          <Label for="import-preset">预设</Label>
          <div class="flex gap-2">
            <Select :model-value="selectedPreset" @update:model-value="onApplyPreset($event as string)">
              <SelectTrigger id="import-preset" class="flex-1">
                <SelectValue placeholder="选择预设" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="preset in props.presets" :key="preset.name" :value="preset.name">
                  {{ preset.name }}
                </SelectItem>
              </SelectContent>
            </Select>
            <Button variant="outline" size="sm" :disabled="!selectedPreset" @click="onRemovePreset">
              删除
            </Button>
          </div>
          <div class="flex gap-2">
            <Input v-model="presetName" placeholder="预设名称" @keydown.enter="onSavePreset" />
            <Button variant="outline" size="sm" @click="onSavePreset">保存</Button>
          </div>
        </div>

        <div v-for="field in CANDIDATE_IMPORT_FIELDS" :key="field.key" class="space-y-2">
          <Label :for="`import-expr-${field.key}`">
            {{ field.label }}
            <span v-if="field.required" class="text-destructive" aria-hidden="true">*</span>
          </Label>
          <Input
            :id="`import-expr-${field.key}`"
            :model-value="props.mapping[field.key]"
            :aria-invalid="Boolean(props.outcome.expressionErrors[field.key])"
            class="font-mono text-xs"
            :placeholder="field.key === 'profile' ? '（可选）' : 'JMESPath 表达式'"
            @update:model-value="emit('expression', field.key, String($event))"
          />
          <p v-if="props.outcome.expressionErrors[field.key]" class="text-sm text-destructive">
            {{ props.outcome.expressionErrors[field.key] }}
          </p>
        </div>

        <div class="space-y-2">
          <Label>可引用的列</Label>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="header in props.sheet.headers"
              :key="header"
              type="button"
              class="inline-flex items-center gap-1 rounded-md border px-2 py-0.5 font-mono text-xs transition-colors hover:bg-accent"
              :aria-label="`复制列名 ${header}`"
              @click="copyColumn(header)"
            >
              <Copy class="h-3 w-3 text-muted-foreground" aria-hidden="true" />"{{ header }}"
            </button>
          </div>
        </div>
      </CardContent>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle>实时预览</CardTitle>
      </CardHeader>
      <CardContent>
        <div v-if="props.existingLoading" class="flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner aria-hidden="true" />
          正在核对既有候选人…
        </div>
        <ErrorAlert
          v-else-if="props.existingError"
          :message="`既有候选人加载失败：${props.existingError}`"
          retry-label="重试"
          @retry="emit('retryExisting')"
        />
        <p v-else-if="props.outcome.mapped.length === 0" class="text-sm text-muted-foreground">
          请先补全必填字段的表达式
        </p>
        <ImportPreviewTable v-else :sheet="props.sheet" :outcome="props.outcome" />
      </CardContent>
    </Card>
  </div>
</template>
