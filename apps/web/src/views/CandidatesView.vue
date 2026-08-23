<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Plus, RefreshCw } from 'lucide-vue-next'
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
import DataTable from '@/components/ui/table/data-table.vue'
import DataTableColumnHeader from '@/components/ui/table/data-table-column-header.vue'
import type { DataTableFeatures } from '@/components/ui/table/features'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const router = useRouter()
const { hasPermission } = useAuth()
// 组合式函数（函数式 ViewModel）：顶层解构，模板直接引用（ref 自动解包）。
const { candidates, statusFilter, keyword, loading, load, create, checkin, update, remove, resetStatus, setStatusFilter, setKeyword } = useCandidates()

// 创建候选人对话框状态
const createOpen = ref(false)
const newName = ref('')
const newProfile = ref('')

// 编辑对话框状态
const editTarget = ref<Candidate | null>(null)
const editName = ref('')
const editProfile = ref('')

// 重置状态对话框状态
const resetTarget = ref<Candidate | null>(null)
const resetStatusValue = ref<CandidateStatus>('NOT_CHECKED_IN')

function openCreate() {
  newName.value = ''
  newProfile.value = ''
  createOpen.value = true
}

async function submitCreate() {
  if (!newName.value.trim()) {
    toast.error('请输入候选人姓名')
    return
  }
  try {
    await create(newName.value.trim(), newProfile.value.trim())
    createOpen.value = false
    toast.success('候选人已创建')
  } catch (e) {
    toast.error((e as Error).message)
  }
}

function openEdit(c: Candidate) {
  editTarget.value = c
  editName.value = c.name
  editProfile.value = c.profile ?? ''
}

async function submitEdit() {
  if (!editTarget.value) return
  if (!editName.value.trim()) {
    toast.error('请输入候选人姓名')
    return
  }
  try {
    await update(editTarget.value.id, editName.value.trim(), editProfile.value.trim())
    editTarget.value = null
    toast.success('已保存')
  } catch (e) {
    toast.error((e as Error).message)
  }
}

async function handleDelete(c: Candidate) {
  const hint = c.room_id ? '该候选人将连带删除其面试记录并解绑房间。' : '该候选人将连带删除其面试记录。'
  if (!window.confirm(`确认删除候选人「${c.name}」？${hint}`)) return
  try {
    await remove(c.id)
    toast.success('候选人已删除')
  } catch (e) {
    toast.error((e as Error).message)
  }
}

function openReset(c: Candidate) {
  resetTarget.value = c
  resetStatusValue.value = c.status
}

/** 重置目标是否为"向前档"（需已有房间）；表单据此禁用不可选的目标并提示。 */
const isForwardTarget = computed(() =>
  resetStatusValue.value === 'ASSIGNED' ||
  resetStatusValue.value === 'IN_PROGRESS' ||
  resetStatusValue.value === 'COMPLETED',
)

async function submitReset() {
  if (!resetTarget.value) return
  if (resetStatusValue.value === resetTarget.value.status) {
    toast.error('目标状态与当前相同')
    return
  }
  if (isForwardTarget.value && !resetTarget.value.room_id) {
    toast.error('该候选人尚未绑定房间，不能重置到已分配/面试中/已结束')
    return
  }
  try {
    await resetStatus(resetTarget.value.id, resetStatusValue.value)
    resetTarget.value = null
    toast.success('状态已重置')
  } catch (e) {
    toast.error((e as Error).message)
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
    toast.error('该候选人尚未分配房间')
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
      h(Button, { size: 'sm', variant: 'destructive', onClick: () => handleDelete(c) }, () => '删除'),
    )
  }
  return h('div', { class: 'flex gap-2' }, buttons)
}

onMounted(() => load())
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="mx-auto max-w-[800px] space-y-4 px-4 py-6">
      <div class="flex items-end justify-between">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight">候选人管理</h1>
        </div>
        <div class="flex items-center gap-2">
          <Button variant="outline" size="icon" aria-label="刷新" @click="load">
            <RefreshCw class="h-4 w-4" />
          </Button>
          <Dialog :open="createOpen" @update:open="createOpen = $event">
            <DialogTrigger as-child>
              <Button v-if="hasPermission(PERMISSIONS.CANDIDATES_CREATE)" @click="openCreate">
                <Plus class="h-4 w-4" />
                新增候选人
              </Button>
            </DialogTrigger>
            <DialogContent class="sm:max-w-[425px]">
              <DialogHeader>
                <DialogTitle>新增候选人</DialogTitle>
              </DialogHeader>
              <div class="grid gap-4">
                <div class="grid gap-2">
                  <Label for="cand-name">姓名</Label>
                  <Input id="cand-name" v-model="newName" placeholder="候选人姓名" />
                </div>
                <div class="grid gap-2">
                  <Label for="cand-profile">个人简介</Label>
                  <Input id="cand-profile" v-model="newProfile" placeholder="技术栈 / 背景（可选）" />
                </div>
              </div>
              <DialogFooter>
                <Button variant="outline" @click="createOpen = false">取消</Button>
                <Button @click="submitCreate">创建</Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </div>
      </div>

      <div class="space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-3">
            <h2 class="text-sm font-semibold">候选人列表</h2>
            <Input
              v-model="keyword"
              placeholder="搜索姓名 / 简介…"
              class="w-56"
              @keydown.enter="setKeyword(keyword)"
            />
          </div>
          <Select :model-value="statusFilter || 'ALL'" @update:model-value="setStatusFilter($event === 'ALL' ? '' : ($event as any))">
            <SelectTrigger class="w-[180px]">
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

        <!-- 空态（加载中留空） -->
        <div v-if="!loading && candidates.length === 0" class="py-12 text-center text-muted-foreground">
          暂无候选人
        </div>

        <!-- 数据表格 -->
        <DataTable v-else :columns="columns" :data="candidates" />
      </div>

      <!-- 编辑对话框 -->
      <Dialog :open="!!editTarget" @update:open="editTarget = $event ? editTarget : null">
        <DialogContent class="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>编辑候选人</DialogTitle>
          </DialogHeader>
          <div class="grid gap-4">
            <div class="grid gap-2">
              <Label for="edit-name">姓名</Label>
              <Input id="edit-name" v-model="editName" />
            </div>
            <div class="grid gap-2">
              <Label for="edit-profile">个人简介</Label>
              <Input id="edit-profile" v-model="editProfile" />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" @click="editTarget = null">取消</Button>
            <Button @click="submitEdit">保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <!-- 重置状态对话框 -->
      <Dialog :open="!!resetTarget" @update:open="resetTarget = $event ? resetTarget : null">
        <DialogContent class="sm:max-w-[380px]">
          <DialogHeader>
            <DialogTitle>重置状态</DialogTitle>
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
                该候选人未绑定房间，不能重置到「已分配 / 面试中 / 已结束」。
              </template>
              <template v-else-if="isForwardTarget">
                重置到该状态需保持当前房间绑定（#{{ resetTarget?.room_id }}）。
              </template>
              <template v-else>
                重置到「未签到 / 已签到待分配」将自动解绑当前房间。
              </template>
            </p>
          </div>
          <DialogFooter>
            <Button variant="outline" @click="resetTarget = null">取消</Button>
            <Button @click="submitReset">重置</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  </div>
</template>
