<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { Plus, RefreshCw, ShieldCheck, UsersRound } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useUsers } from '@/composables/useUsers'
import { useAuth } from '@/composables/useAuth'
import { groupPermissionEntries, PERMISSION_ORDER, permissionGroup, type PermissionGroup } from '@/presenters/permissions'
import type { Role, User } from '@/models'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import DataTable from '@/components/ui/table/data-table.vue'
import type { DataTableFeatures } from '@/components/ui/table/features'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { tabItemVariants } from '@/components/ui/tokens'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import EmptyState from '@/components/app/EmptyState.vue'
import PageShell from '@/components/app/PageShell.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const { users, roles, loading, keyword, load, setKeyword, createUser, updateUser, deleteUser, resetUserPassword, createRole, updateRole, deleteRole } = useUsers()
const { currentUserId } = useAuth()

onMounted(() => void load())

const tab = ref<'users' | 'roles'>('users')

// ---- 用户操作 ----
const userDialogOpen = ref(false)
const editingUser = ref<User | null>(null)
const savingUser = ref(false)
const userForm = ref({ username: '', name: '', password: '', role_ids: [] as number[] })

function openCreateUser() {
  editingUser.value = null
  userForm.value = { username: '', name: '', password: '', role_ids: [] }
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
      await updateUser(editingUser.value.id, { name: userForm.value.name, role_ids: userForm.value.role_ids })
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

// ---- 角色操作 ----
const roleDialogOpen = ref(false)
const editingRole = ref<Role | null>(null)
const savingRole = ref(false)
const roleForm = ref({ name: '', description: '', permissions: [] as string[] })

// 角色删除确认对话框。
const deleteRoleTarget = ref<Role | null>(null)
const deletingRole = ref(false)

function openCreateRole() {
  editingRole.value = null
  roleForm.value = { name: '', description: '', permissions: [] }
  savingRole.value = false
  roleDialogOpen.value = true
}

function openEditRole(r: Role) {
  editingRole.value = r
  roleForm.value = {
    name: r.name,
    description: r.description ?? '',
    permissions: (r.permissions ?? []).map((p) => p.permission),
  }
  savingRole.value = false
  roleDialogOpen.value = true
}

async function submitRole() {
  if (!roleForm.value.name.trim()) {
    toast.error('请输入角色名')
    return
  }
  if (savingRole.value) return
  savingRole.value = true
  try {
    if (editingRole.value) {
      await updateRole(editingRole.value.id, {
        name: roleForm.value.name.trim(),
        description: roleForm.value.description,
        permissions: roleForm.value.permissions,
      })
      toast.success('角色已更新')
    } else {
      await createRole({
        name: roleForm.value.name.trim(),
        description: roleForm.value.description,
        permissions: roleForm.value.permissions,
      })
      toast.success('角色已创建')
    }
    roleDialogOpen.value = false
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    savingRole.value = false
  }
}

async function confirmDeleteRole() {
  if (!deleteRoleTarget.value || deletingRole.value) return
  deletingRole.value = true
  try {
    await deleteRole(deleteRoleTarget.value.id)
    toast.success('角色已删除')
    deleteRoleTarget.value = null
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    deletingRole.value = false
  }
}

function toggleRolePerm(p: string) {
  const idx = roleForm.value.permissions.indexOf(p)
  if (idx >= 0) roleForm.value.permissions.splice(idx, 1)
  else roleForm.value.permissions.push(p)
}

/** 角色的权限按「管理/流程」分组有序展示（View 纯函数）。 */
function rolePerms(r: Role): Record<PermissionGroup, string[]> {
  return groupPermissionEntries((r.permissions ?? []).map((p) => p.permission))
}

/** 全量权限目录按「管理/流程」分组（编辑角色勾选用）。 */
const permGroups = computed<Record<PermissionGroup, string[]>>(() => {
  const out: Record<PermissionGroup, string[]> = { 管理: [], 流程: [] }
  for (const p of PERMISSION_ORDER) {
    const g = permissionGroup(p)
    if (g) out[g].push(p)
  }
  return out
})

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

const roleColumnHelper = createColumnHelper<DataTableFeatures, Role>()
const roleColumns: ColumnDef<DataTableFeatures, Role>[] = roleColumnHelper.columns([
  roleColumnHelper.accessor('name', {
    header: '角色名',
    cell: ({ getValue }) => h('div', { class: 'font-medium' }, getValue()),
  }),
  roleColumnHelper.accessor('description', {
    header: '描述',
    cell: ({ getValue }) => h('div', { class: 'text-muted-foreground' }, getValue() || '-'),
  }),
  roleColumnHelper.accessor('permissions', {
    header: '权限',
    enableSorting: false,
    cell: ({ row }) => {
      const groups = rolePerms(row.original)
      const has = groups['管理'].length || groups['流程'].length
      if (!has) return h('span', { class: 'text-muted-foreground' }, '-')
      return h(
        'div',
        { class: 'flex max-w-[420px] flex-col gap-1.5' },
        (['管理', '流程'] as const).map((g) =>
          groups[g].length
            ? h(
                'div',
                { class: 'flex flex-wrap items-center gap-1' },
                [
                  h('span', { class: 'w-10 shrink-0 text-[11px] text-muted-foreground' }, g),
                  ...groups[g].map((key) =>
                    h(Badge, { variant: 'outline', class: 'font-mono text-[11px]' }, () => key),
                  ),
                ],
              )
            : null,
        ),
      )
    },
  }),
  roleColumnHelper.display({
    id: 'actions',
    header: '操作',
    enableHiding: false,
    cell: ({ row }) =>
      h(
        'div',
        { class: 'flex gap-2' },
        [
          h(Button, { size: 'sm', variant: 'outline', onClick: () => openEditRole(row.original) }, () => '编辑'),
          h(Button, { size: 'sm', variant: 'destructive', onClick: () => (deleteRoleTarget.value = row.original) }, () => '删除'),
        ],
      ),
  }),
])
</script>

<template>
  <PageShell title="面试官管理" description="管理面试官账号、角色与 RBAC 权限。">
    <template #actions>
      <Button variant="outline" size="icon" aria-label="刷新列表" @click="load">
        <RefreshCw :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
      </Button>
    </template>

    <!-- 页签 -->
    <div class="flex items-center gap-1" role="tablist" aria-label="面试官 / 角色">
      <button
        role="tab"
        :aria-selected="tab === 'users'"
        :class="tabItemVariants({ active: tab === 'users' })"
        @click="tab = 'users'"
      >
        面试官
      </button>
      <button
        role="tab"
        :aria-selected="tab === 'roles'"
        :class="tabItemVariants({ active: tab === 'roles' })"
        @click="tab = 'roles'"
      >
        角色
      </button>
    </div>

    <!-- 面试官 -->
    <div v-if="tab === 'users'" class="space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <!-- 搜索框：图标 + 可清空（Enter / 清空均触发检索）。 -->
        <SearchInput
          v-model="keyword"
          placeholder="搜索用户名 / 姓名…"
          @search="setKeyword"
        />
        <Button size="sm" @click="openCreateUser">
          <Plus aria-hidden="true" />
          新增面试官
        </Button>
      </div>

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

    <!-- 角色 -->
    <div v-else class="space-y-4">
      <div class="flex items-center justify-end">
        <Button size="sm" @click="openCreateRole">
          <Plus aria-hidden="true" />
          新增角色
        </Button>
      </div>

      <!-- 加载骨架屏 -->
      <div v-if="loading && roles.length === 0" class="space-y-2" aria-busy="true">
        <Skeleton v-for="i in 3" :key="i" class="h-16 w-full rounded-md" />
      </div>

      <!-- 空态 -->
      <EmptyState v-else-if="roles.length === 0" :icon="ShieldCheck">
        暂无角色，创建角色以分配权限
      </EmptyState>

      <DataTable v-else :columns="roleColumns" :data="roles" />
    </div>

    <!-- 用户对话框 -->
    <Dialog :open="userDialogOpen" @update:open="userDialogOpen = $event">
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>{{ editingUser ? '编辑面试官' : '新增面试官' }}</DialogTitle>
        </DialogHeader>
        <div class="grid gap-4">
          <div class="grid gap-2">
            <Label for="u-username">用户名（登录名，不可修改）</Label>
            <Input id="u-username" v-model="userForm.username" :disabled="!!editingUser" placeholder="登录用户名" autocomplete="off" />
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
            <Label>角色</Label>
            <div v-if="roles.length === 0" class="rounded-md border border-dashed p-3 text-xs text-muted-foreground">
              暂无可选角色，请先在「角色」页签创建。
            </div>
            <div v-else class="grid grid-cols-2 gap-2">
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

    <!-- 角色对话框 -->
    <Dialog :open="roleDialogOpen" @update:open="roleDialogOpen = $event">
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{{ editingRole ? '编辑角色' : '新增角色' }}</DialogTitle>
        </DialogHeader>
        <div class="grid gap-4">
          <div class="grid gap-2">
            <Label for="r-name">角色名</Label>
            <Input id="r-name" v-model="roleForm.name" placeholder="如 auditor" autocomplete="off" />
          </div>
          <div class="grid gap-2">
            <Label for="r-desc">描述</Label>
            <Input id="r-desc" v-model="roleForm.description" placeholder="角色说明（可选）" />
          </div>
          <div class="grid gap-2">
            <Label>权限组</Label>
            <div class="grid max-h-64 gap-2 overflow-y-auto rounded-md border p-3">
              <template v-for="g in ['管理', '流程'] as const" :key="g">
                <div class="space-y-1.5">
                  <p class="text-xs font-medium text-muted-foreground">{{ g }}</p>
                  <div class="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
                    <label
                      v-for="p in permGroups[g]"
                      :key="p"
                      class="flex cursor-pointer items-center gap-2 rounded-sm text-sm transition-colors hover:bg-accent/50 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring"
                    >
                      <input
                        type="checkbox"
                        class="accent-primary"
                        :checked="roleForm.permissions.includes(p)"
                        @change="toggleRolePerm(p)"
                      />
                      <code class="font-mono text-xs">{{ p }}</code>
                    </label>
                  </div>
                </div>
              </template>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" :disabled="savingRole" @click="roleDialogOpen = false">取消</Button>
          <Button :disabled="savingRole" @click="submitRole">
            <Spinner v-if="savingRole" />
            {{ editingRole ? '保存' : '创建' }}
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
          <p class="text-xs text-muted-foreground">重置后该用户当前登录态将立即失效。</p>
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
      :description="
        deleteTarget
          ? `确认删除「${deleteTarget.name || deleteTarget.username}」？其房间席位将被移除，已发消息保留。`
          : ''
      "
      confirm-text="删除"
      destructive
      :loading="deleting"
      @update:open="deleteTarget = $event ? deleteTarget : null"
      @confirm="confirmDelete"
    />

    <!-- 删除角色确认 -->
    <ConfirmDialog
      :open="!!deleteRoleTarget"
      title="删除角色"
      :description="
        deleteRoleTarget
          ? `确认删除角色「${deleteRoleTarget.name}」？仍被用户使用的角色无法删除。`
          : ''
      "
      confirm-text="删除"
      destructive
      :loading="deletingRole"
      @update:open="deleteRoleTarget = $event ? deleteRoleTarget : null"
      @confirm="confirmDeleteRole"
    />
  </PageShell>
</template>
