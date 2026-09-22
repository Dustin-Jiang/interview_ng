<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { Plus, UsersRound } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useUsers } from '@/composables/useUsers'
import { useEntityDialog } from '@/composables/useEntityDialog'
import { useAuth } from '@/composables/useAuth'
import { useConfirmAction } from '@/composables/useConfirmAction'
import type { User } from '@/models'
import { toastError } from '@/lib/toast'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import type { DataTableFeatures } from '@/components/ui/table/features'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import DataTableSection from '@/components/app/DataTableSection.vue'
import FormDialog from '@/components/app/FormDialog.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const { users, roles, departments, loading, keyword, load, setKeyword, createUser, updateUser, deleteUser, resetUserPassword } = useUsers()
const { currentUserId } = useAuth()

onMounted(() => void load())

const emptyText = computed(() => (keyword.value ? '没有匹配的面试官' : '暂无面试官'))

// ---- 用户操作 ----
/** 面试官表单草稿（新增与编辑共用）。 */
interface UserForm {
  username: string
  name: string
  password: string
  role_ids: number[]
  department_id: number | null
}

// 新增 / 编辑面试官对话框（target 为空即新增）
const {
  open: userDialogOpen,
  editing: editingUser,
  form: userForm,
  saving: savingUser,
  openCreate: openCreateUser,
  openEdit: openEditUser,
  submit: submitUser,
} = useEntityDialog<User, UserForm>({
  blank: () => ({
    username: '',
    name: '',
    password: '',
    role_ids: [],
    department_id: departments.value[0]?.id ?? null,
  }),
  toForm: (u) => ({
    username: u.username,
    name: u.name ?? '',
    password: '',
    role_ids: (u.roles ?? []).map((r) => r.id),
    department_id: u.department_id ?? null,
  }),
  validate: (form, target) => {
    if (!form.username.trim()) return '请输入用户名'
    // 初始密码只在新建时设置；编辑改密走「重置密码」。
    if (!target && !form.password) return '请设置初始密码'
    return null
  },
  action: async (form, target) => {
    const body = {
      username: form.username.trim(),
      name: form.name,
      role_ids: form.role_ids,
      department_id: form.department_id,
    }
    if (target) await updateUser(target.id, body)
    else await createUser({ ...body, password: form.password })
  },
  success: (_form, target) => (target ? '已保存' : '面试官已创建'),
})

// 删除面试官确认对话框（替代 window.confirm）。
const {
  target: deleteTarget,
  loading: deleting,
  request: requestDelete,
  onOpenChange: onDeleteOpenChange,
  confirm: confirmDelete,
} = useConfirmAction<User>({
  action: (u) => deleteUser(u.id),
  success: () => '已删除',
})

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
    toastError(e)
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
    buttons.push(h(Button, { size: 'sm', variant: 'destructive', onClick: () => requestDelete(u) }, () => '删除'))
  }
  return h('div', { class: 'flex gap-2' }, buttons)
}
</script>

<template>
  <PageShell title="面试官">
    <template #actions>
      <RefreshButton label="刷新列表" :loading="loading" @click="load" />
      <Button size="sm" @click="openCreateUser">
        <Plus aria-hidden="true" />
        新增面试官
      </Button>
    </template>

    <DataTableSection
      :loading="loading"
      :items="users"
      :columns="userColumns"
      :data="users"
      :empty-text="emptyText"
      :empty-icon="UsersRound"
    >
      <template #toolbar>
        <SearchInput
          v-model="keyword"
          placeholder="搜索用户名 / 姓名…"
          @search="setKeyword"
        />
      </template>
    </DataTableSection>

    <!-- 用户对话框 -->
    <FormDialog
      :open="userDialogOpen"
      :title="editingUser ? '编辑面试官' : '新增面试官'"
      :submit-text="editingUser ? '保存' : '创建'"
      :loading="savingUser"
      @update:open="userDialogOpen = $event"
      @submit="submitUser"
    >
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
        <!-- 手机上两列会把中文角色名挤成多行，故 <sm 单列；每行整块可点且高度 ≥44px。 -->
        <div v-if="roles.length" class="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <label
            v-for="r in roles"
            :key="r.id"
            class="flex cursor-pointer items-center gap-2 rounded-md border p-2 text-sm transition-colors hover:bg-accent/50 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring max-lg:min-h-11"
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
    </FormDialog>

    <!-- 重置密码对话框 -->
    <FormDialog
      :open="!!resetTarget"
      title="重置密码"
      submit-text="重置密码"
      size="sm"
      :loading="resetting"
      :submit-disabled="!newPassword"
      @update:open="resetTarget = $event ? resetTarget : null"
      @submit="submitResetPassword"
    >
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
    </FormDialog>

    <!-- 删除面试官确认 -->
    <ConfirmDialog
      :open="!!deleteTarget"
      title="删除面试官"
      confirm-text="删除"
      destructive
      :loading="deleting"
      @update:open="onDeleteOpenChange"
      @confirm="confirmDelete"
    />
  </PageShell>
</template>
