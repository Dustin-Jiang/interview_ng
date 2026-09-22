<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { Building2, Plus } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useUsers } from '@/composables/useUsers'
import { useConfirmAction } from '@/composables/useConfirmAction'
import { useAuth } from '@/composables/useAuth'
import { admissionApi, candidateApi } from '@/api/http'
import { PERMISSIONS, type AdmissionStatus, type Department } from '@/models'
import { toastError } from '@/lib/toast'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { textCell } from '@/components/ui/table/cells'
import type { DataTableFeatures } from '@/components/ui/table/features'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import DataTableSection from '@/components/app/DataTableSection.vue'
import FormDialog from '@/components/app/FormDialog.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'

const { departments, loading, load, createDepartment, updateDepartment, deleteDepartment } = useUsers()

const { hasPermission } = useAuth()
/** 录取三档统计仅跨部门权限（candidates.browse_all）下能拉齐全量；无权限则不出录取列。 */
const canBrowseAll = computed(() => hasPermission(PERMISSIONS.CANDIDATES_BROWSE_ALL))

type AdmissionTally = Record<AdmissionStatus, number>
/** department_id → 录取三档计数（仅 browse_all 拉取）。 */
const admissionStats = ref(new Map<number, AdmissionTally>())
/** 志愿部门名 → 填报该志愿的候选人数（同名第一/第二志愿只计一次）。 */
const choiceByDept = ref(new Map<string, number>())
/** 统计是否已就绪；失败保持 false → 统计列显示 `-`，不把「没拉到」伪装成 0。 */
const statsReady = ref(false)

/** 拉取录取三档与志愿人数（录取决定无看板事件，随挂载与手动刷新更新）。 */
async function loadStats(): Promise<void> {
  try {
    const [adm, cands] = await Promise.all([
      canBrowseAll.value ? admissionApi.list() : Promise.resolve(null),
      candidateApi.listAll(),
    ])
    if (adm) {
      const tally = new Map<number, AdmissionTally>()
      for (const a of adm.items) {
        const e = tally.get(a.department_id) ?? { admitted: 0, pending: 0, withdrawn: 0 }
        e[a.status] += 1
        tally.set(a.department_id, e)
      }
      admissionStats.value = tally
    }
    const counts = new Map<string, number>()
    for (const c of cands.items) {
      const first = c.first_choice?.trim()
      const second = c.second_choice?.trim()
      if (first) counts.set(first, (counts.get(first) ?? 0) + 1)
      if (second && second !== first) counts.set(second, (counts.get(second) ?? 0) + 1)
    }
    choiceByDept.value = counts
    statsReady.value = true
  } catch {
    // 统计失败不阻塞主表：数字列保持 `-`。
  }
}

function admissionTally(id: number): AdmissionTally | undefined {
  return statsReady.value ? admissionStats.value.get(id) ?? { admitted: 0, pending: 0, withdrawn: 0 } : undefined
}

/** 志愿人数按部门名精确匹配（志愿是文本字段，部门改名后旧文本自然不再计入）。 */
function choiceCount(name: string): number | undefined {
  return statsReady.value ? choiceByDept.value.get(name) ?? 0 : undefined
}

async function reloadAll(): Promise<void> {
  await Promise.all([load(), loadStats()])
}

onMounted(() => void reloadAll())

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

// ---- DataTable 列定义（数字列 tabular-nums，展示口径同候选人管理表） ----

/** 统计数字单元格：未就绪或不适用 → `-`。 */
function statNumber(value: number | null | undefined) {
  return textCell(value == null ? '' : String(value), 'whitespace-nowrap tabular-nums')
}

const deptColumnHelper = createColumnHelper<DataTableFeatures, Department>()
const colName = deptColumnHelper.accessor('name', {
  header: '部门名称',
  cell: ({ getValue }) => h('div', { class: 'font-medium whitespace-nowrap' }, getValue()),
})
const colDescription = deptColumnHelper.accessor('description', {
  header: '描述',
  cell: ({ getValue }) => h('div', { class: 'text-muted-foreground' }, getValue() || '-'),
})
const colExpected = deptColumnHelper.accessor('expected_count', {
  header: '预期人数',
  cell: ({ getValue }) => statNumber(getValue()),
})
const colMembers = deptColumnHelper.accessor('member_count', {
  header: '面试官数',
  cell: ({ getValue }) => statNumber(getValue()),
})
// 志愿人数：填报本部门为第一/第二志愿的候选人（GET /candidates 全量）。
const colChoices = deptColumnHelper.display({
  id: 'stat-choices',
  header: '志愿人数',
  enableSorting: false,
  cell: ({ row }) => statNumber(choiceCount(row.original.name)),
})
// 录取三档（GET /admissions）仅 browse_all 出列。
const colAdmitted = deptColumnHelper.display({
  id: 'stat-admitted',
  header: '已录取',
  enableSorting: false,
  cell: ({ row }) => statNumber(admissionTally(row.original.id)?.admitted),
})
const colPending = deptColumnHelper.display({
  id: 'stat-pending',
  header: '待定',
  enableSorting: false,
  cell: ({ row }) => statNumber(admissionTally(row.original.id)?.pending),
})
const colWithdrawn = deptColumnHelper.display({
  id: 'stat-withdrawn',
  header: '放弃',
  enableSorting: false,
  cell: ({ row }) => statNumber(admissionTally(row.original.id)?.withdrawn),
})
const colActions = deptColumnHelper.display({
  id: 'actions',
  header: '操作',
  enableHiding: false,
  cell: ({ row }) =>
    h(
      'div',
      { class: 'flex gap-2 whitespace-nowrap' },
      [
        h(Button, { size: 'sm', class: 'text-sm', variant: 'outline', onClick: () => openEditDept(row.original) }, () => '编辑'),
        h(Button, { size: 'sm', class: 'text-sm', variant: 'destructive', onClick: () => requestDelete(row.original) }, () => '删除'),
      ],
    ),
})

/** 录取三档需要跨部门全量数据（browse_all）；无权限时不出列，避免把「看不见」显示成 0。 */
const deptColumns = computed<ColumnDef<DataTableFeatures, Department>[]>(
  () =>
    (canBrowseAll.value
      ? [colName, colDescription, colExpected, colMembers, colChoices, colAdmitted, colPending, colWithdrawn, colActions]
      : [colName, colDescription, colExpected, colMembers, colChoices, colActions]) as ColumnDef<DataTableFeatures, Department>[],
)
</script>

<template>
  <PageShell title="部门">
    <template #actions>
      <RefreshButton label="刷新列表" :loading="loading" @click="reloadAll" />
      <Button size="sm" @click="openCreateDept">
        <Plus aria-hidden="true" />
        新增部门
      </Button>
    </template>

    <!-- 表头不换行（数字列 tabular-nums，放不下由容器横向滚动，同其他管理表） -->
    <div class="[&_th]:whitespace-nowrap">
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
    </div>

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
