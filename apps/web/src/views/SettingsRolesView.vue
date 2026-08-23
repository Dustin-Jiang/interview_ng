<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { Plus, RefreshCw, ShieldCheck } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useUsers } from '@/composables/useUsers'
import { groupPermissionEntries, PERMISSION_ORDER, permissionGroup, type PermissionGroup } from '@/presenters/permissions'
import type { Role } from '@/models'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import DataTable from '@/components/ui/table/data-table.vue'
import type { DataTableFeatures } from '@/components/ui/table/features'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import EmptyState from '@/components/app/EmptyState.vue'
import PageShell from '@/components/app/PageShell.vue'

const { roles, loading, load, createRole, updateRole, deleteRole } = useUsers()

onMounted(() => void load())

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
  <PageShell title="角色">
    <template #actions>
      <Button variant="outline" size="icon" aria-label="刷新列表" @click="load">
        <RefreshCw :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
      </Button>
      <Button size="sm" @click="openCreateRole">
        <Plus aria-hidden="true" />
        新增角色
      </Button>
    </template>

    <div class="space-y-4">
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

    <!-- 删除角色确认 -->
    <ConfirmDialog
      :open="!!deleteRoleTarget"
      title="删除角色"
      confirm-text="删除"
      destructive
      :loading="deletingRole"
      @update:open="deleteRoleTarget = $event ? deleteRoleTarget : null"
      @confirm="confirmDeleteRole"
    />
  </PageShell>
</template>
