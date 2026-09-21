<script setup lang="ts">
import { h, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { Building2, Plus } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useUsers } from '@/composables/useUsers'
import { useConfirmAction } from '@/composables/useConfirmAction'
import type { Department } from '@/models'
import { toastError } from '@/lib/toast'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { DataTableFeatures } from '@/components/ui/table/features'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import DataTableSection from '@/components/app/DataTableSection.vue'
import FormDialog from '@/components/app/FormDialog.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'

const { departments, loading, load, createDepartment, updateDepartment, deleteDepartment } = useUsers()

onMounted(() => void load())

// ---- 部门操作 ----
const deptDialogOpen = ref(false)
const editingDept = ref<Department | null>(null)
const savingDept = ref(false)
const deptForm = ref({ name: '', description: '', expected_count: 0 })

const {
  target: deleteTarget,
  loading: deleting,
  request: requestDelete,
  onOpenChange: onDeleteOpenChange,
  confirm: confirmDelete,
} = useConfirmAction<Department>({
  action: (d) => deleteDepartment(d.id),
  success: () => '部门已删除',
})

function openCreateDept() {
  editingDept.value = null
  deptForm.value = { name: '', description: '', expected_count: 0 }
  savingDept.value = false
  deptDialogOpen.value = true
}

function openEditDept(d: Department) {
  editingDept.value = d
  deptForm.value = { name: d.name, description: d.description ?? '', expected_count: d.expected_count ?? 0 }
  savingDept.value = false
  deptDialogOpen.value = true
}

/** 预期人数归一化：非负整数以外返回 null（校验失败）。 */
function normalizeExpectedCount(): number | null {
  const n = Math.floor(Number(deptForm.value.expected_count))
  if (!Number.isFinite(n) || n < 0) return null
  return n
}

async function submitDept() {
  if (!deptForm.value.name.trim()) {
    toast.error('请输入部门名称')
    return
  }
  const expectedCount = normalizeExpectedCount()
  if (expectedCount === null) {
    toast.error('预期人数须为非负整数')
    return
  }
  if (savingDept.value) return
  savingDept.value = true
  try {
    if (editingDept.value) {
      await updateDepartment(editingDept.value.id, {
        name: deptForm.value.name.trim(),
        description: deptForm.value.description,
        expected_count: expectedCount,
      })
      toast.success('部门已更新')
    } else {
      await createDepartment({
        name: deptForm.value.name.trim(),
        description: deptForm.value.description,
        expected_count: expectedCount,
      })
      toast.success('部门已创建')
    }
    deptDialogOpen.value = false
  } catch (e) {
    toastError(e)
  } finally {
    savingDept.value = false
  }
}

// ---- DataTable 列定义 ----
const deptColumnHelper = createColumnHelper<DataTableFeatures, Department>()
const deptColumns: ColumnDef<DataTableFeatures, Department>[] = deptColumnHelper.columns([
  deptColumnHelper.accessor('name', {
    header: '部门名称',
    cell: ({ getValue }) => h('div', { class: 'font-medium' }, getValue()),
  }),
  deptColumnHelper.accessor('description', {
    header: '描述',
    cell: ({ getValue }) => h('div', { class: 'text-muted-foreground' }, getValue() || '-'),
  }),
  deptColumnHelper.accessor('expected_count', {
    header: '预期人数',
    cell: ({ getValue }) => h('span', { class: 'text-muted-foreground' }, String(getValue())),
  }),
  deptColumnHelper.accessor('member_count', {
    header: '面试官数',
    cell: ({ getValue }) => h('span', { class: 'text-muted-foreground' }, String(getValue())),
  }),
  deptColumnHelper.display({
    id: 'actions',
    header: '操作',
    enableHiding: false,
    cell: ({ row }) =>
      h(
        'div',
        { class: 'flex gap-2' },
        [
          h(Button, { size: 'sm', variant: 'outline', onClick: () => openEditDept(row.original) }, () => '编辑'),
          h(Button, { size: 'sm', variant: 'destructive', onClick: () => requestDelete(row.original) }, () => '删除'),
        ],
      ),
  }),
])
</script>

<template>
  <PageShell title="部门">
    <template #actions>
      <RefreshButton label="刷新列表" :loading="loading" @click="load" />
      <Button size="sm" @click="openCreateDept">
        <Plus aria-hidden="true" />
        新增部门
      </Button>
    </template>

    <DataTableSection
      :loading="loading"
      :items="departments"
      :columns="deptColumns"
      :data="departments"
      empty-text="暂无部门，新增部门以划分面试官归属"
      :empty-icon="Building2"
      :skeleton-rows="3"
      skeleton-item-class="h-16 w-full rounded-md"
    />

    <!-- 部门对话框 -->
    <FormDialog
      :open="deptDialogOpen"
      :title="editingDept ? '编辑部门' : '新增部门'"
      :submit-text="editingDept ? '保存' : '创建'"
      size="sm"
      :loading="savingDept"
      @update:open="deptDialogOpen = $event"
      @submit="submitDept"
    >
      <div class="grid gap-2">
        <Label for="d-name">部门名称</Label>
        <Input id="d-name" v-model="deptForm.name" placeholder="如 后端组" autocomplete="off" />
      </div>
      <div class="grid gap-2">
        <Label for="d-desc">描述</Label>
        <Input id="d-desc" v-model="deptForm.description" placeholder="部门说明（可选）" />
      </div>
      <div class="grid gap-2">
        <Label for="d-expected">预期人数</Label>
        <Input
          id="d-expected"
          v-model.number="deptForm.expected_count"
          type="number"
          min="0"
          step="1"
          placeholder="0"
        />
      </div>
    </FormDialog>

    <!-- 删除部门确认 -->
    <ConfirmDialog
      :open="!!deleteTarget"
      title="删除部门"
      confirm-text="删除"
      destructive
      :loading="deleting"
      @update:open="onDeleteOpenChange"
      @confirm="confirmDelete"
    />
  </PageShell>
</template>
