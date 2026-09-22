<!--
  OidcRoleRules —— 「组 → 角色」映射规则表（受控组件：rules 通过 v-model:rules 双向绑定）。
  语义：自上而下逐条求值 JMESPath 表达式，首个命中生效（优先级即行序）；
  上移/下移直接重排数组（保存时下标即 position）。
-->
<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { toast } from 'vue-sonner'
import { ArrowDown, ArrowUp, Plus, ShieldCheck } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useConfirmAction } from '@/composables/useConfirmAction'
import type { OidcRulePayload, Role } from '@/models'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { DataTableFeatures } from '@/components/ui/table/features'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import DataTableSection from '@/components/app/DataTableSection.vue'
import FormDialog from '@/components/app/FormDialog.vue'

const props = defineProps<{
  rules: OidcRulePayload[]
  roles: readonly Role[]
}>()

const emit = defineEmits<{
  'update:rules': [rules: OidcRulePayload[]]
}>()

function roleName(id: number): string {
  return props.roles.find((r) => r.id === id)?.name ?? '未知角色'
}

/** 上移/下移：交换相邻两条（首/末行按钮禁用）。 */
function move(rule: OidcRulePayload, delta: number): void {
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
} = useConfirmAction<OidcRulePayload>({
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
const form = ref({ expression: '', roleId: '' })

function openCreate(): void {
  editingIndex.value = null
  form.value = { expression: '', roleId: '' }
  dialogOpen.value = true
}

function openEdit(index: number): void {
  const rule = props.rules[index]
  editingIndex.value = index
  form.value = { expression: rule.expression, roleId: String(rule.role_id) }
  dialogOpen.value = true
}

function submit(): void {
  const expression = form.value.expression.trim()
  const roleId = Number(form.value.roleId)
  if (!expression || !roleId) {
    toast.error('请填写表达式并选择角色')
    return
  }
  const rule: OidcRulePayload = { expression, role_id: roleId }
  const next = [...props.rules]
  if (editingIndex.value === null) next.push(rule)
  else next[editingIndex.value] = rule
  emit('update:rules', next)
  dialogOpen.value = false
}

// ---- DataTable 列定义 ----
const helper = createColumnHelper<DataTableFeatures, OidcRulePayload>()

const colPosition = helper.display({
  id: 'position',
  header: '优先级',
  enableSorting: false,
  cell: ({ row }) => h('div', { class: 'tabular-nums text-muted-foreground' }, String(row.index + 1)),
})
const colExpression = helper.accessor('expression', {
  header: 'JMESPath 表达式',
  enableSorting: false,
  cell: ({ getValue }) => h('code', { class: 'font-mono text-xs break-all' }, getValue()),
})
const colRole = helper.display({
  id: 'role',
  header: '角色',
  enableSorting: false,
  cell: ({ row }) => h('div', { class: 'whitespace-nowrap' }, roleName(row.original.role_id)),
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

const columns = computed<ColumnDef<DataTableFeatures, OidcRulePayload>[]>(
  () => [colPosition, colExpression, colRole, colActions] as ColumnDef<DataTableFeatures, OidcRulePayload>[],
)
</script>

<template>
  <div class="[&_th]:whitespace-nowrap">
    <DataTableSection
      :loading="false"
      :items="props.rules"
      :columns="columns"
      :data="props.rules"
      empty-text="暂无规则，匹配不到规则的用户将被拒绝登录"
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
    :submit-disabled="!form.expression.trim() || !form.roleId"
    @update:open="dialogOpen = $event"
    @submit="submit"
  >
    <div class="grid gap-2">
      <Label for="rule-expression">JMESPath 表达式</Label>
      <Input
        id="rule-expression"
        v-model="form.expression"
        placeholder="groups[?starts_with(@, 'interview-')] | [0]"
        class="font-mono text-xs"
      />
    </div>
    <div class="grid gap-2">
      <Label for="rule-role">角色</Label>
      <Select :model-value="form.roleId" @update:model-value="form.roleId = String($event)">
        <SelectTrigger id="rule-role" class="w-full">
          <SelectValue placeholder="选择角色" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem v-for="r in props.roles" :key="r.id" :value="String(r.id)">{{ r.name }}</SelectItem>
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
