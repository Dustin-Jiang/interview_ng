<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { Plus, RefreshCw } from 'lucide-vue-next'
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
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'

const { users, roles, loading, keyword, load, setKeyword, createUser, updateUser, deleteUser, resetUserPassword, createRole, updateRole, deleteRole } = useUsers()
const { currentUserId } = useAuth()

onMounted(() => void load())

const tab = ref<'users' | 'roles'>('users')

// ---- 用户操作 ----
const userDialogOpen = ref(false)
const editingUser = ref<User | null>(null)
const userForm = ref({ username: '', name: '', password: '', role_ids: [] as number[] })

function openCreateUser() {
  editingUser.value = null
  userForm.value = { username: '', name: '', password: '', role_ids: [] }
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
  userDialogOpen.value = true
}

async function submitUser() {
  if (!userForm.value.username.trim()) {
    toast.error('请输入用户名')
    return
  }
  try {
    if (editingUser.value) {
      await updateUser(editingUser.value.id, { name: userForm.value.name, role_ids: userForm.value.role_ids })
      toast.success('已保存')
    } else {
      if (!userForm.value.password) {
        toast.error('请设置初始密码')
        return
      }
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
  }
}

async function handleDeleteUser(u: User) {
  if (!window.confirm(`确认删除面试官「${u.name || u.username}」？其房间席位将被移除，已发消息保留。`)) return
  try {
    await deleteUser(u.id)
    toast.success('已删除')
  } catch (e) {
    toast.error((e as Error).message)
  }
}

async function handleResetPassword(u: User) {
  const pass = window.prompt(`为「${u.name || u.username}」设置新密码：`)
  if (pass == null || !pass) return
  try {
    await resetUserPassword(u.id, pass)
    toast.success('密码已重置，该用户旧登录已失效')
  } catch (e) {
    toast.error((e as Error).message)
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
const roleForm = ref({ name: '', description: '', permissions: [] as string[] })

function openCreateRole() {
  editingRole.value = null
  roleForm.value = { name: '', description: '', permissions: [] }
  roleDialogOpen.value = true
}

function openEditRole(r: Role) {
  editingRole.value = r
  roleForm.value = {
    name: r.name,
    description: r.description ?? '',
    permissions: (r.permissions ?? []).map((p) => p.permission),
  }
  roleDialogOpen.value = true
}

async function submitRole() {
  if (!roleForm.value.name.trim()) {
    toast.error('请输入角色名')
    return
  }
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
  }
}

async function handleDeleteRole(r: Role) {
  if (!window.confirm(`确认删除角色「${r.name}」？仍被用户使用的角色无法删除。`)) return
  try {
    await deleteRole(r.id)
    toast.success('角色已删除')
  } catch (e) {
    toast.error((e as Error).message)
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
    h(Button, { size: 'sm', variant: 'outline', onClick: () => handleResetPassword(u) }, () => '重置密码'),
  ]
  if (!isSelf(u)) {
    buttons.push(h(Button, { size: 'sm', variant: 'destructive', onClick: () => handleDeleteUser(u) }, () => '删除'))
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
          h(Button, { size: 'sm', variant: 'destructive', onClick: () => handleDeleteRole(row.original) }, () => '删除'),
        ],
      ),
  }),
])
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="mx-auto max-w-[800px] space-y-4 px-4 py-6">
      <div class="flex items-end justify-between">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight">面试官管理</h1>
        </div>
        <Button variant="outline" size="icon" aria-label="刷新" @click="load">
          <RefreshCw class="h-4 w-4" />
        </Button>
      </div>

      <!-- 页签 -->
      <div class="flex items-center gap-1">
        <button
          class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
          :class="tab === 'users' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent'"
          @click="tab = 'users'"
        >
          面试官
        </button>
        <button
          class="rounded-md px-3 py-1.5 text-sm font-medium transition-colors"
          :class="tab === 'roles' ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent'"
          @click="tab = 'roles'"
        >
          角色
        </button>
      </div>

      <!-- 面试官 -->
      <div v-if="tab === 'users'" class="space-y-4">
        <div class="flex items-center justify-between">
          <Input
            v-model="keyword"
            placeholder="搜索用户名 / 姓名…"
            class="w-56"
            @keydown.enter="setKeyword(keyword)"
          />
          <Button size="sm" @click="openCreateUser">
            <Plus class="h-4 w-4" />
            新增面试官
          </Button>
        </div>
        <DataTable :columns="userColumns" :data="users" />
      </div>

      <!-- 角色 -->
      <div v-else class="space-y-4">
        <div class="flex items-center justify-end">
          <Button size="sm" @click="openCreateRole">
            <Plus class="h-4 w-4" />
            新增角色
          </Button>
        </div>
        <DataTable :columns="roleColumns" :data="roles" />
      </div>

      <!-- 用户对话框 -->
      <Dialog :open="userDialogOpen" @update:open="userDialogOpen = $event">
        <DialogContent class="sm:max-w-[440px]">
          <DialogHeader>
            <DialogTitle>{{ editingUser ? '编辑面试官' : '新增面试官' }}</DialogTitle>
          </DialogHeader>
          <div class="grid gap-4">
            <div class="grid gap-2">
              <Label for="u-username">用户名（登录名，不可修改）</Label>
              <Input id="u-username" v-model="userForm.username" :disabled="!!editingUser" placeholder="登录用户名" />
            </div>
            <div class="grid gap-2">
              <Label for="u-name">姓名</Label>
              <Input id="u-name" v-model="userForm.name" placeholder="显示姓名" />
            </div>
            <div v-if="!editingUser" class="grid gap-2">
              <Label for="u-password">初始密码</Label>
              <Input id="u-password" v-model="userForm.password" type="password" placeholder="初始密码" />
            </div>
            <div class="grid gap-2">
              <Label>角色</Label>
              <div class="grid grid-cols-2 gap-2">
                <label
                  v-for="r in roles"
                  :key="r.id"
                  class="flex items-center gap-2 rounded-md border p-2 text-sm"
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
            <Button variant="outline" @click="userDialogOpen = false">取消</Button>
            <Button @click="submitUser">{{ editingUser ? '保存' : '创建' }}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <!-- 角色对话框 -->
      <Dialog :open="roleDialogOpen" @update:open="roleDialogOpen = $event">
        <DialogContent class="sm:max-w-[480px]">
          <DialogHeader>
            <DialogTitle>{{ editingRole ? '编辑角色' : '新增角色' }}</DialogTitle>
          </DialogHeader>
          <div class="grid gap-4">
            <div class="grid gap-2">
              <Label for="r-name">角色名</Label>
              <Input id="r-name" v-model="roleForm.name" placeholder="如 auditor" />
            </div>
            <div class="grid gap-2">
              <Label for="r-desc">描述</Label>
              <Input id="r-desc" v-model="roleForm.description" placeholder="角色说明（可选）" />
            </div>
            <div class="grid gap-2">
              <Label>权限组</Label>
              <div class="grid gap-2 rounded-md border p-3">
                <template v-for="g in ['管理', '流程'] as const" :key="g">
                  <div class="space-y-1.5">
                    <p class="text-xs font-medium text-muted-foreground">{{ g }}</p>
                    <div class="grid grid-cols-1 gap-1.5">
                      <label
                        v-for="p in permGroups[g]"
                        :key="p"
                        class="flex items-center gap-2 text-sm"
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
            <Button variant="outline" @click="roleDialogOpen = false">取消</Button>
            <Button @click="submitRole">{{ editingRole ? '保存' : '创建' }}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  </div>
</template>
