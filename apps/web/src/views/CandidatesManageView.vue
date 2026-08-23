<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { ShieldAlert, Plus, RefreshCw, UsersRound } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useCandidates } from '@/composables/useCandidates'
import { useAuth } from '@/composables/useAuth'
import { CANDIDATE_STATUSES, PERMISSIONS, type Candidate, type CandidateStatus } from '@/models'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { formatDateTime } from '@/lib/format'

// --- shadcn-vue UI ---
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import DataTable from '@/components/ui/table/data-table.vue'
import DataTableColumnHeader from '@/components/ui/table/data-table-column-header.vue'
import type { DataTableFeatures } from '@/components/ui/table/features'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import EmptyState from '@/components/app/EmptyState.vue'
import PageShell from '@/components/app/PageShell.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const router = useRouter()
const { hasPermission } = useAuth()
// 组合式函数（函数式 ViewModel）：顶层解构，模板直接引用（ref 自动解包）。
const { candidates, statusFilter, keyword, loading, load, create, checkin, update, remove, resetStatus, setStatusFilter, setKeyword } = useCandidates()

/** 管理页可见性：持有任一候选人管理类权限（对普通只读用户不可见）。 */
const canAccessManage = computed(
  () =>
    hasPermission(PERMISSIONS.CANDIDATES_MANAGE) ||
    hasPermission(PERMISSIONS.CANDIDATES_CREATE) ||
    hasPermission(PERMISSIONS.CANDIDATES_CHECKIN),
)

// 创建候选人对话框状态
const createOpen = ref(false)
const newName = ref('')
const newProfile = ref('')
const creating = ref(false)

// 编辑对话框状态
const editTarget = ref<Candidate | null>(null)
const editName = ref('')
const editProfile = ref('')
const editing = ref(false)

// 删除确认对话框状态
const deleteTarget = ref<Candidate | null>(null)
const deleting = ref(false)

// 重置状态对话框状态
const resetTarget = ref<Candidate | null>(null)
const resetStatusValue = ref<CandidateStatus>('NOT_CHECKED_IN')
const resetting = ref(false)

function openCreate() {
  newName.value = ''
  newProfile.value = ''
  creating.value = false
  createOpen.value = true
}

async function submitCreate() {
  if (!newName.value.trim()) {
    toast.error('请输入候选人姓名')
    return
  }
  if (creating.value) return
  creating.value = true
  try {
    await create(newName.value.trim(), newProfile.value.trim())
    createOpen.value = false
    toast.success('候选人已创建')
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    creating.value = false
  }
}

function openEdit(c: Candidate) {
  editTarget.value = c
  editName.value = c.name
  editProfile.value = c.profile ?? ''
  editing.value = false
}

async function submitEdit() {
  if (!editTarget.value || editing.value) return
  if (!editName.value.trim()) {
    toast.error('请输入候选人姓名')
    return
  }
  editing.value = true
  try {
    await update(editTarget.value.id, editName.value.trim(), editProfile.value.trim())
    editTarget.value = null
    toast.success('已保存')
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    editing.value = false
  }
}

async function confirmDelete() {
  if (!deleteTarget.value || deleting.value) return
  deleting.value = true
  try {
    await remove(deleteTarget.value.id)
    toast.success('候选人已删除')
    deleteTarget.value = null
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    deleting.value = false
  }
}

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
    toast.error('该候选人尚未绑定房间，不能重置到待面试/面试中/已结束')
    return
  }
  resetting.value = true
  try {
    await resetStatus(resetTarget.value.id, resetStatusValue.value)
    resetTarget.value = null
    toast.success('状态已重置')
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    resetting.value = false
  }
}

async function handleCheckin(c: Candidate) {
  try {
    await checkin(c.id)
    toast.success('签到成功')
  } catch (e) {
    toast.error((e as Error).message)
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
const columnHelper = createColumnHelper<DataTableFeatures, Candidate>()
const columns: ColumnDef<DataTableFeatures, Candidate>[] = columnHelper.columns([
  columnHelper.accessor('id', {
    header: 'ID',
    enableSorting: false,
    cell: ({ getValue }) => h('div', { class: 'font-mono text-xs' }, String(getValue())),
  }),
  columnHelper.accessor('name', {
    header: ({ column }) => h(DataTableColumnHeader, { column: column as any, title: '姓名' }),
  }),
  columnHelper.accessor('status', {
    header: '状态',
    enableSorting: false,
    cell: ({ row }) =>
      h(
        Badge,
        { variant: STATUS_PRESENTATION[row.original.status].badge },
        () => STATUS_PRESENTATION[row.original.status].label,
      ),
  }),
  columnHelper.accessor('room_id', {
    header: '房间',
    enableSorting: false,
    cell: ({ row }) => {
      const roomId = row.original.room_id
      if (!roomId) return h('span', { class: 'text-muted-foreground' }, '-')
      return h(Button, { variant: 'link', class: 'h-auto p-0', onClick: () => goRoom(roomId) }, () => `#${roomId}`)
    },
  }),
  columnHelper.accessor('profile', {
    header: '简介',
    enableSorting: false,
    cell: ({ getValue }) =>
      h('div', { class: 'max-w-[220px] truncate text-muted-foreground' }, getValue() || '-'),
  }),
  columnHelper.accessor('created_at', {
    header: ({ column }) => h(DataTableColumnHeader, { column: column as any, title: '创建时间' }),
    cell: ({ getValue }) => h('div', { class: 'text-muted-foreground' }, formatDateTime(String(getValue()))),
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
    buttons.push(h(Button, { size: 'sm', variant: 'outline', onClick: () => handleCheckin(c) }, () => '签到'))
  }
  if (hasPermission(PERMISSIONS.CANDIDATES_MANAGE)) {
    buttons.push(
      h(Button, { size: 'sm', variant: 'outline', onClick: () => openEdit(c) }, () => '编辑'),
      h(Button, { size: 'sm', variant: 'outline', onClick: () => openReset(c) }, () => '重置状态'),
      // 打开确认对话框（ConfirmDialog），不再使用 window.confirm。
      h(Button, { size: 'sm', variant: 'destructive', onClick: () => (deleteTarget.value = c) }, () => '删除'),
    )
  }
  return h('div', { class: 'flex gap-2' }, buttons)
}

onMounted(() => {
  if (canAccessManage.value) void load()
})
</script>

<template>
  <PageShell title="候选人管理" description="签到候选人、维护资料与状态重置（管理员功能）。">
    <template v-if="canAccessManage" #actions>
      <Button variant="outline" size="icon" aria-label="刷新候选人列表" @click="load">
        <RefreshCw :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
      </Button>
      <Dialog :open="createOpen" @update:open="createOpen = $event">
        <DialogTrigger as-child>
          <Button v-if="hasPermission(PERMISSIONS.CANDIDATES_CREATE)" @click="openCreate">
            <Plus aria-hidden="true" />
            新增候选人
          </Button>
        </DialogTrigger>
        <DialogContent size="md">
          <DialogHeader>
            <DialogTitle>新增候选人</DialogTitle>
          </DialogHeader>
          <div class="grid gap-4">
            <div class="grid gap-2">
              <Label for="cand-name">姓名</Label>
              <Input id="cand-name" v-model="newName" placeholder="候选人姓名" @keydown.enter="submitCreate" />
            </div>
            <div class="grid gap-2">
              <Label for="cand-profile">个人简介</Label>
              <Textarea id="cand-profile" v-model="newProfile" rows="3" placeholder="技术栈 / 背景（可选）" />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" :disabled="creating" @click="createOpen = false">取消</Button>
            <Button :disabled="creating" @click="submitCreate">
              <Spinner v-if="creating" />
              创建
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </template>

    <!-- 普通用户不可见：无任一管理权限时仅显示提示 -->
    <EmptyState v-if="!canAccessManage" bare :icon="ShieldAlert" class="py-16">
      无候选人管理权限
      <template #hint>该界面仅对持有候选人管理权限的用户可见</template>
    </EmptyState>

    <template v-else>
    <div class="space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h2 class="text-sm font-semibold">候选人列表</h2>
        <div class="flex flex-wrap items-center gap-3">
          <!-- 搜索框：图标 + 可清空（Enter / 清空均触发检索）。 -->
          <SearchInput
            v-model="keyword"
            placeholder="搜索姓名 / 简介…"
            @search="setKeyword"
          />
          <Select :model-value="statusFilter || 'ALL'" @update:model-value="setStatusFilter($event === 'ALL' ? '' : ($event as any))">
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
      </div>

      <!-- 加载骨架屏（首屏无数据时） -->
      <div v-if="loading && candidates.length === 0" class="space-y-2" aria-busy="true">
        <Skeleton v-for="i in 5" :key="i" class="h-12 w-full rounded-md" />
      </div>

      <!-- 空态：说明 + 引导动作 -->
      <EmptyState v-else-if="candidates.length === 0" :icon="UsersRound">
        {{ keyword ? '没有匹配的候选人' : '暂无候选人' }}
        <template v-if="!keyword && hasPermission(PERMISSIONS.CANDIDATES_CREATE)" #hint>
          点击右上角「新增候选人」开始使用
        </template>
      </EmptyState>

      <!-- 数据表格 -->
      <DataTable v-else :columns="columns" :data="candidates" />
    </div>

    <!-- 编辑对话框 -->
    <Dialog :open="!!editTarget" @update:open="editTarget = $event ? editTarget : null">
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>编辑候选人</DialogTitle>
        </DialogHeader>
        <div class="grid gap-4">
          <div class="grid gap-2">
            <Label for="edit-name">姓名</Label>
            <Input id="edit-name" v-model="editName" @keydown.enter="submitEdit" />
          </div>
          <div class="grid gap-2">
            <Label for="edit-profile">个人简介</Label>
            <Textarea id="edit-profile" v-model="editProfile" rows="3" placeholder="技术栈 / 背景（可选）" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" :disabled="editing" @click="editTarget = null">取消</Button>
          <Button :disabled="editing" @click="submitEdit">
            <Spinner v-if="editing" />
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- 重置状态对话框 -->
    <Dialog :open="!!resetTarget" @update:open="resetTarget = $event ? resetTarget : null">
      <DialogContent size="sm">
        <DialogHeader>
          <DialogTitle>重置状态</DialogTitle>
          <DialogDescription v-if="resetTarget">
            将「{{ resetTarget.name }}」从「{{ STATUS_PRESENTATION[resetTarget.status].label }}」重置到目标状态。
          </DialogDescription>
        </DialogHeader>
        <div class="grid gap-3">
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
          <p class="text-xs text-muted-foreground">
            <template v-if="isForwardTarget && !resetTarget?.room_id">
              该候选人未绑定房间，不能重置到「待面试 / 面试中 / 已结束」。
            </template>
            <template v-else-if="isForwardTarget">
              重置到该状态需保持当前房间绑定（#{{ resetTarget?.room_id }}）。
            </template>
            <template v-else>
              重置到「未签到 / 排队中」将自动解绑当前房间。
            </template>
          </p>
        </div>
        <DialogFooter>
          <Button variant="outline" :disabled="resetting" @click="resetTarget = null">取消</Button>
          <Button :disabled="resetting" @click="submitReset">
            <Spinner v-if="resetting" />
            重置
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- 删除候选人确认 -->
    <ConfirmDialog
      :open="!!deleteTarget"
      title="删除候选人"
      :description="
        deleteTarget
          ? `确认删除「${deleteTarget.name}」？${deleteTarget.room_id ? '将解绑其所在房间，' : ''}面试记录一并删除，操作不可撤销。`
          : ''
      "
      confirm-text="删除"
      destructive
      :loading="deleting"
      @update:open="deleteTarget = $event ? deleteTarget : null"
      @confirm="confirmDelete"
    />
    </template>
  </PageShell>
</template>
