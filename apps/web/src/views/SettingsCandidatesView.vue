<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Pencil, Plus, UsersRound } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useCandidates } from '@/composables/useCandidates'
import { useEntityDialog } from '@/composables/useEntityDialog'
import { useRooms } from '@/composables/useRooms'
import { useAuth } from '@/composables/useAuth'
import { useConfirmAction } from '@/composables/useConfirmAction'
import { PERMISSIONS, type Candidate, type CandidateInfoPayload, type CandidateStatus } from '@/models'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { blankCandidateInfo, candidateInfoOf } from '@/domain/candidateInfo'
import { normalizeStudentNo } from '@/domain/studentNo'
import { formatDateTime } from '@/lib/format'
import { toastError } from '@/lib/toast'

// --- shadcn-vue UI ---
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import DataTableColumnHeader from '@/components/ui/table/data-table-column-header.vue'
import { textCell } from '@/components/ui/table/cells'
import type { DataTableFeatures } from '@/components/ui/table/features'
import CandidateFormFields from '@/components/app/CandidateFormFields.vue'
import CandidatePreferenceDialog from '@/components/app/CandidatePreferenceDialog.vue'
import CandidateStatusSelect from '@/components/app/CandidateStatusSelect.vue'
import ClampText from '@/components/app/ClampText.vue'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import DataTableSection from '@/components/app/DataTableSection.vue'
import FormDialog from '@/components/app/FormDialog.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const router = useRouter()
const { hasPermission } = useAuth()
// 组合式函数（函数式 ViewModel）：顶层解构，模板直接引用（ref 自动解包）。
const { candidates, statusFilter, keyword, loading, load, create, checkin, update, remove, resetStatus, setStatusFilter, setKeyword } = useCandidates()
// 房间列显示名字（候选人只带 room_id）；未命名显示「未命名」。名单取自 useRooms（共享数据源）。
const { roomLabelOf, load: loadRooms } = useRooms()

const emptyText = computed(() => (keyword.value ? '没有匹配的候选人' : '暂无候选人'))

// 新增 / 编辑候选人对话框（同一对话框两态：target 为空即新增）
const {
  open: candidateDialogOpen,
  editing: editingCandidate,
  form: candidateForm,
  saving: candidateSaving,
  openCreate: openCreateCandidate,
  openEdit: openEditCandidate,
  submit: submitCandidate,
} = useEntityDialog<Candidate, CandidateInfoPayload>({
  blank: blankCandidateInfo,
  toForm: candidateInfoOf,
  validate: (info) => {
    const studentNo = normalizeStudentNo(info.student_no)
    if ('error' in studentNo) return studentNo.error
    // 归一化后写回草稿：提交体即屏幕上的值（全角折半角、去首尾空白）。
    info.student_no = studentNo.value
    if (!info.name.trim()) return '请输入候选人姓名'
    return null
  },
  action: async (info, target) => {
    const payload = { ...info, name: info.name.trim() }
    if (target) await update(target.id, payload)
    else await create(payload)
  },
  success: (_info, target) => (target ? '已保存' : '候选人已创建'),
})

// 重置状态对话框（同一套「目标 + 表单」状态：表单即目标档）
const {
  open: resetDialogOpen,
  form: resetForm,
  saving: resetting,
  openEdit: openReset,
  submit: submitReset,
} = useEntityDialog<Candidate, CandidateStatus | ''>({
  blank: () => 'NOT_CHECKED_IN',
  toForm: (c) => c.status,
  validate: (status, target) => {
    if (!target) return null
    if (status === target.status) return '目标状态与当前相同'
    if (isForwardStatus(status) && !target.room_id) {
      return '该候选人尚未绑定房间，不能重置到待面试/面试中/面试已结束'
    }
    return null
  },
  action: async (status, target) => {
    if (target && status) await resetStatus(target.id, status)
  },
  success: () => '状态已重置',
})

/** 重置目标是否为「向前档」（重置到这些档要求候选人已绑定房间）。 */
function isForwardStatus(status: CandidateStatus | ''): boolean {
  return status === 'ASSIGNED' || status === 'IN_PROGRESS' || status === 'COMPLETED'
}

// 志愿与调剂对话框状态（独立小权限：无 candidates.manage 的面试官也可改这三项）
const preferenceTarget = ref<Candidate | null>(null)

function openPreferences(c: Candidate) {
  preferenceTarget.value = c
}

// 删除确认对话框（替代 window.confirm）。
const {
  target: deleteTarget,
  loading: deleting,
  request: requestDelete,
  onOpenChange: onDeleteOpenChange,
  confirm: confirmDelete,
} = useConfirmAction<Candidate>({
  action: (c) => remove(c.id),
  success: () => '候选人已删除',
})

async function handleCheckin(c: Candidate) {
  try {
    await checkin(c.id)
    toast.success('签到成功')
  } catch (e) {
    toastError(e)
  }
}

function goRoom(roomId?: number) {
  if (!roomId) {
    toast.error('该候选人尚未进入房间')
    return
  }
  router.push({ name: 'room', params: { roomId: String(roomId) } })
}

// ---- DataTable 列定义（h() 渲染，闭包捕获视图处理函数） ----
// 展示口径与导入预览一致：空值 `-` 占位、数字类 mono、各列 nowrap（放不下由容器
// 横向滚动）、个人简介 3 行截断 + Popover 看全文；状态/房间/操作为本页专属列。
const columnHelper = createColumnHelper<DataTableFeatures, Candidate>()
const columns: ColumnDef<DataTableFeatures, Candidate>[] = columnHelper.columns([
  columnHelper.accessor('id', {
    header: 'ID',
    enableSorting: false,
    cell: ({ getValue }) => h('div', { class: 'whitespace-nowrap tabular-nums' }, String(getValue())),
  }),
  columnHelper.accessor('student_no', {
    header: ({ column }) => h(DataTableColumnHeader, { column: column as any, title: '学号' }),
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap tabular-nums'),
  }),
  columnHelper.accessor('name', {
    header: ({ column }) => h(DataTableColumnHeader, { column: column as any, title: '姓名' }),
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap'),
  }),
  columnHelper.accessor('status', {
    header: '状态',
    enableSorting: false,
    cell: ({ row }) =>
      h(
        Badge,
        { variant: STATUS_PRESENTATION[row.original.status].badge, class: 'text-sm' },
        () => STATUS_PRESENTATION[row.original.status].label,
      ),
  }),
  columnHelper.accessor('room_id', {
    header: '房间',
    enableSorting: false,
    cell: ({ row }) => {
      const roomId = row.original.room_id
      if (!roomId) return h('span', { class: 'text-muted-foreground' }, '-')
      return h(Button, { variant: 'link', class: 'h-auto p-0 whitespace-nowrap', onClick: () => goRoom(roomId) }, () => roomLabelOf(roomId))
    },
  }),
  columnHelper.accessor('first_choice', {
    header: '第一志愿',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap'),
  }),
  columnHelper.accessor('second_choice', {
    header: '第二志愿',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap'),
  }),
  columnHelper.accessor('accept_adjust', {
    header: '接受调剂',
    enableSorting: false,
    cell: ({ getValue }) => h('div', { class: 'whitespace-nowrap' }, getValue() ? '接受' : '不接受'),
  }),
  columnHelper.accessor('phone', {
    header: '手机号',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap tabular-nums'),
  }),
  columnHelper.accessor('qq', {
    header: 'QQ号',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap tabular-nums'),
  }),
  columnHelper.accessor('email', {
    header: '邮箱',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap tabular-nums'),
  }),
  columnHelper.accessor('profile', {
    header: '个人简介',
    enableSorting: false,
    cell: ({ getValue }) => {
      const value = String(getValue() ?? '')
      if (!value) return h('span', { class: 'text-muted-foreground' }, '-')
      return h(ClampText, { text: value, lines: 3, class: 'min-w-56 text-muted-foreground' })
    },
  }),
  columnHelper.accessor('created_at', {
    header: ({ column }) => h(DataTableColumnHeader, { column: column as any, title: '创建时间' }),
    cell: ({ getValue }) =>
      h('div', { class: 'whitespace-nowrap text-muted-foreground' }, formatDateTime(String(getValue()))),
  }),
  columnHelper.display({
    id: 'actions',
    header: '操作',
    enableHiding: false,
    cell: ({ row }) => renderActions(row.original),
  }),
])

/** 操作列渲染（按权限与状态显隐，均为有边框按钮；删除用 destructive）。 */
function renderActions(c: Candidate) {
  const buttons: ReturnType<typeof h>[] = []
  if (c.status === 'NOT_CHECKED_IN' && hasPermission(PERMISSIONS.CANDIDATES_CHECKIN)) {
    buttons.push(h(Button, { size: 'sm', class: 'text-sm', variant: 'outline', onClick: () => handleCheckin(c) }, () => '签到'))
  }
  if (hasPermission(PERMISSIONS.CANDIDATES_PREFERENCES)) {
    buttons.push(
      h(Button, { size: 'sm', class: 'text-sm gap-1', variant: 'outline', onClick: () => openPreferences(c) },
        () => [h(Pencil, { 'aria-hidden': 'true' }), '改志愿']),
    )
  }
  if (hasPermission(PERMISSIONS.CANDIDATES_MANAGE)) {
    buttons.push(
      h(Button, { size: 'sm', class: 'text-sm', variant: 'outline', onClick: () => openEditCandidate(c) }, () => '编辑'),
      h(Button, { size: 'sm', class: 'text-sm', variant: 'outline', onClick: () => openReset(c) }, () => '重置状态'),
      // 打开确认对话框（ConfirmDialog），不再使用 window.confirm。
      h(Button, { size: 'sm', class: 'text-sm', variant: 'destructive', onClick: () => requestDelete(c) }, () => '删除'),
    )
  }
  // 手机上卡片宽仅 ~300px（5 个按钮一行放不下），故 <md 允许换行；≥md 表格里保持一行。
  return h('div', { class: 'flex gap-2 whitespace-nowrap max-md:flex-wrap' }, buttons)
}

onMounted(() => {
  void load()
  // 房间名映射与名单分开拉：两条数据源各自刷新（房间 CRUD 事件由 useRooms 自己订阅）。
  void loadRooms()
})
</script>

<template>
  <PageShell title="候选人管理">
    <template #actions>
      <RefreshButton label="刷新候选人列表" :loading="loading" @click="load" />
      <Button v-if="hasPermission(PERMISSIONS.CANDIDATES_CREATE)" @click="openCreateCandidate">
        <Plus aria-hidden="true" />
        新增候选人
      </Button>
    </template>

    <DataTableSection
      title="候选人列表"
      nowrap-headers
      :loading="loading"
      :items="candidates"
      :columns="columns"
      :data="candidates"
      :empty-text="emptyText"
      :empty-icon="UsersRound"
    >
      <template #toolbar>
        <div class="flex flex-wrap items-center gap-3">
          <!-- 搜索框：图标 + 可清空（Enter / 清空均触发检索）。 -->
          <SearchInput
            v-model="keyword"
            placeholder="搜索学号 / 姓名 / 简介…"
            @search="setKeyword"
          />
          <CandidateStatusSelect
            :model-value="statusFilter"
            allow-all
            trigger-class="w-full sm:w-[180px]"
            @update:model-value="setStatusFilter"
          />
        </div>
      </template>
    </DataTableSection>

    <!-- 新增 / 编辑候选人对话框（两态共用同一表单） -->
    <FormDialog
      :open="candidateDialogOpen"
      :title="editingCandidate ? '编辑候选人' : '新增候选人'"
      :submit-text="editingCandidate ? '保存' : '创建'"
      size="lg"
      :loading="candidateSaving"
      @update:open="candidateDialogOpen = $event"
      @submit="submitCandidate"
    >
      <CandidateFormFields
        v-model:info="candidateForm"
        id-prefix="cand"
        @submit="submitCandidate"
      />
    </FormDialog>

    <!-- 重置状态对话框 -->
    <FormDialog
      :open="resetDialogOpen"
      title="重置状态"
      submit-text="重置"
      size="sm"
      :loading="resetting"
      @update:open="resetDialogOpen = $event"
      @submit="submitReset"
    >
      <div class="grid gap-2">
        <Label for="reset-status">目标状态</Label>
        <CandidateStatusSelect v-model="resetForm" trigger-id="reset-status" />
      </div>
    </FormDialog>

    <!-- 志愿与调剂对话框（独立小权限） -->
    <CandidatePreferenceDialog
      :open="!!preferenceTarget"
      :candidate="preferenceTarget"
      @update:open="preferenceTarget = $event ? preferenceTarget : null"
      @saved="load"
    />

    <!-- 删除候选人确认 -->
    <ConfirmDialog
      :open="!!deleteTarget"
      title="删除候选人"
      confirm-text="删除"
      destructive
      :loading="deleting"
      @update:open="onDeleteOpenChange"
      @confirm="confirmDelete"
    />
  </PageShell>
</template>
