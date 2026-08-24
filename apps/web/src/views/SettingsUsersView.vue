<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { Plus, RefreshCw, UsersRound } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useUsers } from '@/composables/useUsers'
import { useAuth } from '@/composables/useAuth'
import type { User } from '@/models'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import DataTable from '@/components/ui/table/data-table.vue'
import type { DataTableFeatures } from '@/components/ui/table/features'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import EmptyState from '@/components/app/EmptyState.vue'
import PageShell from '@/components/app/PageShell.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const { users, roles, departments, loading, keyword, load, setKeyword, createUser, updateUser, deleteUser, resetUserPassword } = useUsers()
const { currentUserId } = useAuth()

onMounted(() => void load())

// ---- 用户操作 ----
const userDialogOpen = ref(false)
const editingUser = ref<User | null>(null)
const savingUser = ref(false)
const userForm = ref({ username: '', name: '', password: '', role_ids: [] as number[], department_id: null as number | null })

function openCreateUser() {
  editingUser.value = null
  userForm.value = { username: '', name: '', password: '', role_ids: [], department_id: departments.value[0]?.id ?? null }
  savingUser.value = false
  userDialogOpen.value = true
}

function openEditUser(u: User) {
  editingUser.value = u
  userForm.value = {
    username: u.username,
    name: u.name ?? '',
    password: '',
    role_ids: (u.roles ?? []).map((r) => r.id),
    department_id: u.department_id ?? null,
  }
  savingUser.value = false
  userDialogOpen.value = true
}

async function submitUser() {
  if (!userForm.value.username.trim()) {
    toast.error('请输入用户名')
    return
  }
  if (savingUser.value) return
  try {
    if (editingUser.value) {
      savingUser.value = true
      await updateUser(editingUser.value.id, {
        username: userForm.value.username.trim(),
        name: userForm.value.name,
        role_ids: userForm.value.role_ids,
        department_id: userForm.value.department_id,
      })
      toast.success('已保存')
    } else {
      if (!userForm.value.password) {
        toast.error('请设置初始密码')
        return
      }
      savingUser.value = true
      await createUser({
        username: userForm.value.username.trim(),
        name: userForm.value.name,
        password: userForm.value.password,
        role_ids: userForm.value.role_ids,
        department_id: userForm.value.department_id,
      })
      toast.success('面试官已创建')
    }
    userDialogOpen.value = false
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    savingUser.value = false
  }
}

// 删除面试官确认对话框（替代 window.confirm）。
const deleteTarget = ref<User | null>(null)
const deleting = ref(false)

async function confirmDelete() {
  if (!deleteTarget.value || deleting.value) return
  deleting.value = true
  try {
    await deleteUser(deleteTarget.value.id)
    toast.success('已删除')
    deleteTarget.value = null
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    deleting.value = false
  }
}

// 重置密码对话框（替代 window.prompt）：显式输入 + 校验。
const resetTarget = ref<User | null>(null)
const newPassword = ref('')
const resetting = ref(false)

function openResetPassword(u: User) {
  resetTarget.value = u
  newPassword.value = ''
  resetting.value = false
}

async function submitResetPassword() {
  if (!resetTarget.value || resetting.value) return
  if (!newPassword.value) {
    toast.error('请输入新密码')
    return
  }
  resetting.value = true
  try {
    await resetUserPassword(resetTarget.value.id, newPassword.value)
    toast.success('密码已重置，该用户旧登录已失效')
    resetTarget.value = null
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    resetting.value = false
  }
}

function toggleUserRole(rid: number) {
  const idx = userForm.value.role_ids.indexOf(rid)
  if (idx >= 0) userForm.value.role_ids.splice(idx, 1)
  else userForm.value.role_ids.push(rid)
}

const isSelf = (u: User) => u.id === currentUserId.value

// ---- DataTable 列定义 ----
const userColumnHelper = createColumnHelper<DataTableFeatures, User>()
const userColumns: ColumnDef<DataTableFeatures, User>[] = userColumnHelper.columns([
  userColumnHelper.accessor('id', {
    header: 'ID',
    enableSorting: false,
    cell: ({ getValue }) => h('div', { class: 'font-mono text-xs' }, String(getValue())),
  }),
  userColumnHelper.accessor('username', {
    header: '用户名',
    cell: ({ getValue }) => h('div', { class: 'font-medium' }, getValue()),
  }),
  userColumnHelper.accessor('name', {
    header: '姓名',
    cell: ({ getValue }) => getValue() || '-',
  }),
  userColumnHelper.accessor('department', {
    header: '部门',
    enableSorting: false,
    cell: ({ row }) => {
      const d = row.original.department
      if (!d) return h('span', { class: 'text-muted-foreground' }, '-')
      return h(Badge, { variant: 'secondary' }, () => d.name)
    },
  }),
  userColumnHelper.accessor('roles', {
    header: '角色',
    enableSorting: false,
    cell: ({ row }) => {
      const rs = row.original.roles ?? []
      if (!rs.length) return h('span', { class: 'text-muted-foreground' }, '-')
      return h(
        'div',
        { class: 'flex flex-wrap gap-1' },
        rs.map((r) => h(Badge, { variant: 'secondary' }, () => r.name)),
      )
    },
  }),
  userColumnHelper.display({
    id: 'actions',
    header: '操作',
    enableHiding: false,
    cell: ({ row }) => renderUserActions(row.original),
  }),
])

/** 面试官操作列（编辑 / 重置密码 / 删除；删除不可作用于自己）。 */
function renderUserActions(u: User) {
  const buttons: ReturnType<typeof h>[] = [
    h(Button, { size: 'sm', variant: 'outline', onClick: () => openEditUser(u) }, () => '编辑'),
    h(Button, { size: 'sm', variant: 'outline', onClick: () => openResetPassword(u) }, () => '重置密码'),
  ]
  if (!isSelf(u)) {
    buttons.push(h(Button, { size: 'sm', variant: 'destructive', onClick: () => (deleteTarget.value = u) }, () => '删除'))
  }
  return h('div', { class: 'flex gap-2' }, buttons)
}
</script>

<template>
  <PageShell title="面试官">
    <template #actions>
      <Button variant="outline" size="icon" aria-label="刷新列表" @click="load">
        <RefreshCw :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
      </Button>
      <Button size="sm" @click="openCreateUser">
        <Plus aria-hidden="true" />
        新增面试官
      </Button>
    </template>

    <div class="space-y-4">
      <SearchInput
        v-model="keyword"
        placeholder="搜索用户名 / 姓名…"
        @search="setKeyword"
      />

      <!-- 加载骨架屏 -->
      <div v-if="loading && users.length === 0" class="space-y-2" aria-busy="true">
        <Skeleton v-for="i in 5" :key="i" class="h-12 w-full rounded-md" />
      </div>

      <!-- 空态 -->
      <EmptyState v-else-if="users.length === 0" :icon="UsersRound">
        {{ keyword ? '没有匹配的面试官' : '暂无面试官' }}
      </EmptyState>

      <DataTable v-else :columns="userColumns" :data="users" />
    </div>

    <!-- 用户对话框 -->
    <Dialog :open="userDialogOpen" @update:open="userDialogOpen = $event">
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>{{ editingUser ? '编辑面试官' : '新增面试官' }}</DialogTitle>
        </DialogHeader>
        <div class="grid gap-4">
          <div class="grid gap-2">
            <Label for="u-username">用户名（登录名）</Label>
            <Input id="u-username" v-model="userForm.username" placeholder="登录用户名" autocomplete="off" />
          </div>
          <div class="grid gap-2">
            <Label for="u-name">姓名</Label>
            <Input id="u-name" v-model="userForm.name" placeholder="显示姓名" />
          </div>
          <div v-if="!editingUser" class="grid gap-2">
            <Label for="u-password">初始密码</Label>
            <Input id="u-password" v-model="userForm.password" type="password" placeholder="初始密码" autocomplete="new-password" />
          </div>
          <div class="grid gap-2">
            <Label for="u-department">部门</Label>
            <Select :model-value="userForm.department_id ? String(userForm.department_id) : ''" @update:model-value="userForm.department_id = $event ? Number($event) : null">
              <SelectTrigger id="u-department" class="w-full">
                <SelectValue placeholder="选择部门" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem v-for="d in departments" :key="d.id" :value="String(d.id)">
                  {{ d.name }}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div class="grid gap-2">
            <Label>角色</Label>
            <div v-if="roles.length" class="grid grid-cols-2 gap-2">
              <label
                v-for="r in roles"
                :key="r.id"
                class="flex cursor-pointer items-center gap-2 rounded-md border p-2 text-sm transition-colors hover:bg-accent/50 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring"
              >
                <input
                  type="checkbox"
                  class="accent-primary"
                  :checked="userForm.role_ids.includes(r.id)"
                  @change="toggleUserRole(r.id)"
                />
                {{ r.name }}
              </label>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" :disabled="savingUser" @click="userDialogOpen = false">取消</Button>
          <Button :disabled="savingUser" @click="submitUser">
            <Spinner v-if="savingUser" />
            {{ editingUser ? '保存' : '创建' }}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- 重置密码对话框 -->
    <Dialog :open="!!resetTarget" @update:open="resetTarget = $event ? resetTarget : null">
      <DialogContent size="sm">
        <DialogHeader>
          <DialogTitle>重置密码</DialogTitle>
        </DialogHeader>
        <div class="grid gap-2">
          <Label for="reset-pass">新密码（为「{{ resetTarget?.name || resetTarget?.username }}」设置）</Label>
          <Input
            id="reset-pass"
            v-model="newPassword"
            type="password"
            placeholder="输入新密码"
            autocomplete="new-password"
            @keydown.enter="submitResetPassword"
          />
        </div>
        <DialogFooter>
          <Button variant="outline" :disabled="resetting" @click="resetTarget = null">取消</Button>
          <Button :disabled="resetting || !newPassword" @click="submitResetPassword">
            <Spinner v-if="resetting" />
            重置密码
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <!-- 删除面试官确认 -->
    <ConfirmDialog
      :open="!!deleteTarget"
      title="删除面试官"
      confirm-text="删除"
      destructive
      :loading="deleting"
      @update:open="deleteTarget = $event ? deleteTarget : null"
      @confirm="confirmDelete"
    />
  </PageShell>
</template>
