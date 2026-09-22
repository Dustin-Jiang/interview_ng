<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Pencil, Plus, UsersRound } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useCandidates } from '@/composables/useCandidates'
import { useRoomNames } from '@/composables/useRoomNames'
import { useAuth } from '@/composables/useAuth'
import { useConfirmAction } from '@/composables/useConfirmAction'
import { CANDIDATE_STATUSES, PERMISSIONS, type Candidate, type CandidateInfoPayload, type CandidateStatus } from '@/models'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { normalizeStudentNo } from '@/domain/studentNo'
import { formatDateTime } from '@/lib/format'
import { toastError } from '@/lib/toast'

// --- shadcn-vue UI ---
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import DataTableColumnHeader from '@/components/ui/table/data-table-column-header.vue'
import { textCell } from '@/components/ui/table/cells'
import type { DataTableFeatures } from '@/components/ui/table/features'
import CandidateFormFields from '@/components/app/CandidateFormFields.vue'
import CandidatePreferenceDialog from '@/components/app/CandidatePreferenceDialog.vue'
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
// 房间列显示名字（候选人只带 room_id）；未命名显示「未命名」。
const { roomLabelOf } = useRoomNames()

const emptyText = computed(() => (keyword.value ? '没有匹配的候选人' : '暂无候选人'))

// 创建候选人对话框状态
const createOpen = ref(false)
const newInfo = ref<CandidateInfoPayload>(blankInfo())
const creating = ref(false)

// 编辑对话框状态
const editTarget = ref<Candidate | null>(null)
const editInfo = ref<CandidateInfoPayload>(blankInfo())
const editing = ref(false)

/** 空资料表单（除必填外全部留空）。 */
function blankInfo(): CandidateInfoPayload {
  return {
    student_no: '',
    name: '',
    profile: '',
    first_choice: '',
    second_choice: '',
    accept_adjust: false,
    phone: '',
    qq: '',
    email: '',
  }
}

// 重置状态对话框状态
const resetTarget = ref<Candidate | null>(null)
const resetStatusValue = ref<CandidateStatus>('NOT_CHECKED_IN')
const resetting = ref(false)

// 志愿与调剂对话框状态（独立小权限：无 candidates.manage 的面试官也可改这三项）
const preferenceTarget = ref<Candidate | null>(null)

function openPreferences(c: Candidate) {
  preferenceTarget.value = c
}

function openCreate() {
  newInfo.value = blankInfo()
  creating.value = false
  createOpen.value = true
}

async function submitCreate() {
  const studentNo = normalizeStudentNo(newInfo.value.student_no)
  if ('error' in studentNo) {
    toast.error(studentNo.error)
    return
  }
  if (!newInfo.value.name?.trim()) {
    toast.error('请输入候选人姓名')
    return
  }
  if (creating.value) return
  creating.value = true
  try {
    await create({ ...newInfo.value, student_no: studentNo.value, name: newInfo.value.name.trim() })
    createOpen.value = false
    toast.success('候选人已创建')
  } catch (e) {
    toastError(e)
  } finally {
    creating.value = false
  }
}

function openEdit(c: Candidate) {
  editTarget.value = c
  editInfo.value = {
    student_no: c.student_no,
    name: c.name,
    profile: c.profile ?? '',
    first_choice: c.first_choice ?? '',
    second_choice: c.second_choice ?? '',
    accept_adjust: c.accept_adjust ?? false,
    phone: c.phone ?? '',
    qq: c.qq ?? '',
    email: c.email ?? '',
  }
  editing.value = false
}

async function submitEdit() {
  if (!editTarget.value || editing.value) return
  const studentNo = normalizeStudentNo(editInfo.value.student_no)
  if ('error' in studentNo) {
    toast.error(studentNo.error)
    return
  }
  if (!editInfo.value.name?.trim()) {
    toast.error('请输入候选人姓名')
    return
  }
  editing.value = true
  try {
    await update(editTarget.value.id, {
      ...editInfo.value,
      student_no: studentNo.value,
      name: editInfo.value.name.trim(),
    })
    editTarget.value = null
    toast.success('已保存')
  } catch (e) {
    toastError(e)
  } finally {
    editing.value = false
  }
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

function openReset(c: Candidate) {
  resetTarget.value = c
  resetStatusValue.value = c.status
  resetting.value = false
}

/** 重置目标是否为"向前档"（需已有房间）；表单据此禁用不可选的目标并提示。 */
const isForwardTarget = computed(() =>
  resetStatusValue.value === 'ASSIGNED' ||
  resetStatusValue.value === 'IN_PROGRESS' ||
  resetStatusValue.value === 'COMPLETED',
)

async function submitReset() {
  if (!resetTarget.value || resetting.value) return
  if (resetStatusValue.value === resetTarget.value.status) {
    toast.error('目标状态与当前相同')
    return
  }
  if (isForwardTarget.value && !resetTarget.value.room_id) {
    toast.error('该候选人尚未绑定房间，不能重置到待面试/面试中/面试已结束')
    return
  }
  resetting.value = true
  try {
    await resetStatus(resetTarget.value.id, resetStatusValue.value)
    resetTarget.value = null
    toast.success('状态已重置')
  } catch (e) {
    toastError(e)
  } finally {
    resetting.value = false
  }
}

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
      h(Button, { size: 'sm', class: 'text-sm', variant: 'outline', onClick: () => openEdit(c) }, () => '编辑'),
      h(Button, { size: 'sm', class: 'text-sm', variant: 'outline', onClick: () => openReset(c) }, () => '重置状态'),
      // 打开确认对话框（ConfirmDialog），不再使用 window.confirm。
      h(Button, { size: 'sm', class: 'text-sm', variant: 'destructive', onClick: () => requestDelete(c) }, () => '删除'),
    )
  }
  return h('div', { class: 'flex gap-2 whitespace-nowrap' }, buttons)
}

onMounted(() => {
  void load()
})
</script>

<template>
  <PageShell title="候选人管理">
    <template #actions>
      <RefreshButton label="刷新候选人列表" :loading="loading" @click="load" />
      <FormDialog
        :open="createOpen"
        title="新增候选人"
        size="lg"
        submit-text="创建"
        :loading="creating"
        @update:open="createOpen = $event"
        @submit="submitCreate"
      >
        <template #trigger>
          <Button v-if="hasPermission(PERMISSIONS.CANDIDATES_CREATE)" @click="openCreate">
            <Plus aria-hidden="true" />
            新增候选人
          </Button>
        </template>

        <CandidateFormFields
          v-model:info="newInfo"
          id-prefix="cand-create"
          @submit="submitCreate"
        />
      </FormDialog>
    </template>

    <!-- 表头不换行（各列内容 nowrap + 容器横向滚动，与导入预览一致） -->
    <div class="[&_th]:whitespace-nowrap">
      <DataTableSection
        title="候选人列表"
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
            <Select :model-value="statusFilter || 'ALL'" @update:model-value="setStatusFilter($event === 'ALL' ? '' : ($event as CandidateStatus))">
              <SelectTrigger class="w-[180px]" aria-label="按状态筛选">
                <SelectValue placeholder="全部状态" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="ALL">全部状态</SelectItem>
                <SelectItem v-for="s in CANDIDATE_STATUSES" :key="s" :value="s">
                  {{ STATUS_PRESENTATION[s].label }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
        </template>
      </DataTableSection>
    </div>

    <!-- 编辑对话框 -->
    <FormDialog
      :open="!!editTarget"
      title="编辑候选人"
      size="lg"
      :loading="editing"
      @update:open="editTarget = $event ? editTarget : null"
      @submit="submitEdit"
    >
      <CandidateFormFields
        v-model:info="editInfo"
        id-prefix="cand-edit"
        @submit="submitEdit"
      />
    </FormDialog>

    <!-- 重置状态对话框 -->
    <FormDialog
      :open="!!resetTarget"
      title="重置状态"
      submit-text="重置"
      size="sm"
      :loading="resetting"
      @update:open="resetTarget = $event ? resetTarget : null"
      @submit="submitReset"
    >
      <div class="grid gap-2">
        <Label for="reset-status">目标状态</Label>
        <Select :model-value="resetStatusValue" @update:model-value="resetStatusValue = $event as CandidateStatus">
          <SelectTrigger id="reset-status" class="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem v-for="s in CANDIDATE_STATUSES" :key="s" :value="s">
              {{ STATUS_PRESENTATION[s].label }}
            </SelectItem>
          </SelectContent>
        </Select>
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
