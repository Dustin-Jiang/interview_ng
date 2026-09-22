<script setup lang="ts">
import { computed, h, onMounted } from 'vue'
import { Plus, ShieldCheck } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useUsers } from '@/composables/useUsers'
import { useEntityDialog } from '@/composables/useEntityDialog'
import { useConfirmAction } from '@/composables/useConfirmAction'
import { groupPermissionEntries, PERMISSION_ORDER, permissionGroup, type PermissionGroup } from '@/presenters/permissions'
import type { Role } from '@/models'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { DataTableFeatures } from '@/components/ui/table/features'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import DataTableSection from '@/components/app/DataTableSection.vue'
import FormDialog from '@/components/app/FormDialog.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'

const { roles, loading, load, createRole, updateRole, deleteRole } = useUsers()

onMounted(() => void load())

// ---- 角色操作 ----
/** 角色表单草稿（新增与编辑共用）。 */
interface RoleForm {
  name: string
  description: string
  permissions: string[]
}

// 新增 / 编辑角色对话框（target 为空即新增）
const {
  open: roleDialogOpen,
  editing: editingRole,
  form: roleForm,
  saving: savingRole,
  openCreate: openCreateRole,
  openEdit: openEditRole,
  submit: submitRole,
} = useEntityDialog<Role, RoleForm>({
  blank: () => ({ name: '', description: '', permissions: [] }),
  toForm: (r) => ({
    name: r.name,
    description: r.description ?? '',
    permissions: (r.permissions ?? []).map((p) => p.permission),
  }),
  validate: (form) => (form.name.trim() ? null : '请输入角色名'),
  action: async (form, target) => {
    const body = {
      name: form.name.trim(),
      description: form.description,
      permissions: form.permissions,
    }
    if (target) await updateRole(target.id, body)
    else await createRole(body)
  },
  success: (_form, target) => (target ? '角色已更新' : '角色已创建'),
})

// 角色删除确认对话框。
const {
  target: deleteTarget,
  loading: deleting,
  request: requestDelete,
  onOpenChange: onDeleteOpenChange,
  confirm: confirmDelete,
} = useConfirmAction<Role>({
  action: (r) => deleteRole(r.id),
  success: () => '角色已删除',
})

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

// ---- DataTable 列定义 ----
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
        // 手机上卡片宽度有限，放开 420px 上限（否则窄卡片里 Badge 被挤）；≥md 表格里保持原限宽。
        'div',
        { class: 'flex max-w-none flex-col gap-1.5 md:max-w-[420px]' },
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
          h(Button, { size: 'sm', variant: 'destructive', onClick: () => requestDelete(row.original) }, () => '删除'),
        ],
      ),
  }),
])
</script>

<template>
  <PageShell title="角色">
    <template #actions>
      <RefreshButton label="刷新列表" :loading="loading" @click="load" />
      <Button size="sm" @click="openCreateRole">
        <Plus aria-hidden="true" />
        新增角色
      </Button>
    </template>

    <DataTableSection
      :loading="loading"
      :items="roles"
      :columns="roleColumns"
      :data="roles"
      empty-text="暂无角色，创建角色以分配权限"
      :empty-icon="ShieldCheck"
      :skeleton-rows="3"
      skeleton-item-class="h-16 w-full rounded-md"
    />

    <!-- 角色对话框 -->
    <FormDialog
      :open="roleDialogOpen"
      :title="editingRole ? '编辑角色' : '新增角色'"
      :submit-text="editingRole ? '保存' : '创建'"
      size="lg"
      :loading="savingRole"
      @update:open="roleDialogOpen = $event"
      @submit="submitRole"
    >
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
                  class="flex cursor-pointer items-center gap-2 rounded-sm text-sm transition-colors hover:bg-accent/50 has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring max-lg:min-h-11"
                >
                  <input
                    type="checkbox"
                    class="accent-primary"
                    :checked="roleForm.permissions.includes(p)"
                    @change="toggleRolePerm(p)"
                  />
                  <code class="min-w-0 break-all font-mono text-xs">{{ p }}</code>
                </label>
              </div>
            </div>
          </template>
        </div>
      </div>
    </FormDialog>

    <!-- 删除角色确认 -->
    <ConfirmDialog
      :open="!!deleteTarget"
      title="删除角色"
      confirm-text="删除"
      destructive
      :loading="deleting"
      @update:open="onDeleteOpenChange"
      @confirm="confirmDelete"
    />
  </PageShell>
</template>
