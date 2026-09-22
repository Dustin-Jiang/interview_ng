<!--
  OidcRuleTable —— OIDC 映射规则表（「组 → 角色」与「组 → 部门」共用；受控组件：v-model:rules）。
  语义：自上而下逐条求值 JMESPath 表达式，首个命中生效（优先级即行序）；
  上移/下移直接重排数组（保存时下标即 position）。
  两类规则只差「目标」字段名（role_id / department_id），故存取由调用方的
  targetOf / makeRule 提供，本组件不关心具体字段名。
-->
<script setup lang="ts" generic="T extends { expression: string }">
import { computed, h, ref, useId } from 'vue'
import { toast } from 'vue-sonner'
import { ArrowDown, ArrowUp, Plus, ShieldCheck } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useConfirmAction } from '@/composables/useConfirmAction'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { DataTableFeatures } from '@/components/ui/table/features'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import DataTableSection from '@/components/app/DataTableSection.vue'
import FormDialog from '@/components/app/FormDialog.vue'

const props = defineProps<{
  rules: T[]
  /** 可选目标（角色 / 部门）列表：id 存进规则，label 用于展示与下拉。 */
  targets: readonly { id: number; label: string }[]
  /** 目标字段的显示名（「角色」/「部门」）。 */
  targetLabel: string
  /** 空表提示语（两类规则的后果不同，由调用方给定）。 */
  emptyText: string
  /** 读一条规则的目标 id。 */
  targetOf: (rule: T) => number
  /** 由表达式与目标 id 造一条规则。 */
  makeRule: (expression: string, targetId: number) => T
}>()

const emit = defineEmits<{
  'update:rules': [rules: T[]]
}>()

/** 同一页可能挂两个规则表，表单控件 id 必须唯一（否则 Label 的 for 会指向另一个表的输入框）。 */
const fieldId = useId()

function targetName(id: number): string {
  return props.targets.find((t) => t.id === id)?.label ?? `未知${props.targetLabel}`
}

/** 上移/下移：交换相邻两条（首/末行按钮禁用）。 */
function move(rule: T, delta: number): void {
  const next = [...props.rules]
  const from = next.indexOf(rule)
  const to = from + delta
  if (from < 0 || to < 0 || to >= next.length) return
  ;[next[from], next[to]] = [next[to], next[from]]
  emit('update:rules', next)
}

const {
  target: deleteTarget,
  loading: deleting,
  request: requestDelete,
  onOpenChange: onDeleteOpenChange,
  confirm: confirmDelete,
} = useConfirmAction<T>({
  action: async (rule) => {
    emit(
      'update:rules',
      props.rules.filter((r) => r !== rule),
    )
  },
  success: () => '规则已删除',
})

// ---- 新增/编辑对话框 ----
const dialogOpen = ref(false)
/** 编辑中的行下标；null = 新增（追加到末尾）。 */
const editingIndex = ref<number | null>(null)
const form = ref({ expression: '', targetId: '' })

function openCreate(): void {
  editingIndex.value = null
  form.value = { expression: '', targetId: '' }
  dialogOpen.value = true
}

function openEdit(index: number): void {
  const rule = props.rules[index]
  editingIndex.value = index
  form.value = { expression: rule.expression, targetId: String(props.targetOf(rule)) }
  dialogOpen.value = true
}

function submit(): void {
  const expression = form.value.expression.trim()
  const targetId = Number(form.value.targetId)
  if (!expression || !targetId) {
    toast.error(`请填写表达式并选择${props.targetLabel}`)
    return
  }
  const rule = props.makeRule(expression, targetId)
  const next = [...props.rules]
  if (editingIndex.value === null) next.push(rule)
  else next[editingIndex.value] = rule
  emit('update:rules', next)
  dialogOpen.value = false
}

// ---- DataTable 列定义 ----
const helper = createColumnHelper<DataTableFeatures, T>()

const colPosition = helper.display({
  id: 'position',
  header: '优先级',
  enableSorting: false,
  cell: ({ row }) => h('div', { class: 'tabular-nums text-muted-foreground' }, String(row.index + 1)),
})
// 表达式列用**取值函数**而非字段名：字段名重载要求 `'expression'` 是具体类型的键，
// 泛型 T 在组件内部尚未解析（TS 只能确认 `T extends { expression: string }`）。
const colExpression = helper.accessor((row: T) => row.expression, {
  id: 'expression',
  header: 'JMESPath 表达式',
  enableSorting: false,
  cell: ({ getValue }) => h('code', { class: 'font-mono text-xs break-all' }, getValue()),
})
const colTarget = helper.display({
  id: 'target',
  header: props.targetLabel,
  enableSorting: false,
  cell: ({ row }) => h('div', { class: 'whitespace-nowrap' }, targetName(props.targetOf(row.original))),
})
const colActions = helper.display({
  id: 'actions',
  header: '操作',
  enableSorting: false,
  enableHiding: false,
  cell: ({ row }) =>
    h('div', { class: 'flex gap-2 whitespace-nowrap' }, [
      h(
        Button,
        {
          size: 'sm',
          variant: 'ghost',
          disabled: row.index === 0,
          'aria-label': '上移',
          onClick: () => move(row.original, -1),
        },
        () => h(ArrowUp),
      ),
      h(
        Button,
        {
          size: 'sm',
          variant: 'ghost',
          disabled: row.index === props.rules.length - 1,
          'aria-label': '下移',
          onClick: () => move(row.original, 1),
        },
        () => h(ArrowDown),
      ),
      h(
        Button,
        { size: 'sm', variant: 'outline', onClick: () => openEdit(row.index) },
        () => '编辑',
      ),
      h(
        Button,
        { size: 'sm', variant: 'destructive', onClick: () => requestDelete(row.original) },
        () => '删除',
      ),
    ]),
})

const columns = computed<ColumnDef<DataTableFeatures, T>[]>(
  () => [colPosition, colExpression, colTarget, colActions] as ColumnDef<DataTableFeatures, T>[],
)
</script>

<template>
  <div class="[&_th]:whitespace-nowrap">
    <DataTableSection
      :loading="false"
      :items="props.rules"
      :columns="columns"
      :data="props.rules"
      :empty-text="props.emptyText"
      :empty-icon="ShieldCheck"
      :skeleton-rows="2"
    >
      <template #toolbar>
        <Button size="sm" @click="openCreate">
          <Plus aria-hidden="true" />
          新增规则
        </Button>
      </template>
    </DataTableSection>
  </div>

  <FormDialog
    :open="dialogOpen"
    :title="editingIndex === null ? '新增规则' : '编辑规则'"
    size="md"
    :submit-disabled="!form.expression.trim() || !form.targetId"
    @update:open="dialogOpen = $event"
    @submit="submit"
  >
    <div class="grid gap-2">
      <Label :for="`${fieldId}-expression`">JMESPath 表达式</Label>
      <Input
        :id="`${fieldId}-expression`"
        v-model="form.expression"
        placeholder="groups[?starts_with(@, 'interview-')] | [0]"
        class="font-mono text-xs"
      />
    </div>
    <div class="grid gap-2">
      <Label :for="`${fieldId}-target`">{{ props.targetLabel }}</Label>
      <Select :model-value="form.targetId" @update:model-value="form.targetId = String($event)">
        <SelectTrigger :id="`${fieldId}-target`" class="w-full">
          <SelectValue :placeholder="`选择${props.targetLabel}`" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="t in props.targets" :key="t.id" :value="String(t.id)">{{ t.label }}</SelectItem>
        </SelectContent>
      </Select>
    </div>
  </FormDialog>

  <ConfirmDialog
    :open="!!deleteTarget"
    title="删除规则"
    confirm-text="删除"
    destructive
    :loading="deleting"
    @update:open="onDeleteOpenChange"
    @confirm="confirmDelete"
  />
</template>
