/**
 * useUsers —— 面试官与角色、部门管理组合式函数（函数式 ViewModel，Q28：角色管理门控于 users.manage）。
 * 用户列表自持；角色 / 部门名单取自 `useRoles` / `useDepartments` 两个共享数据源——
 * 本页是它们的**写入方**，增删改落库后调各自的 `load()` 强制刷新，其余读取方共用同一份缓存。
 */
import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { departmentApi, roleApi, userApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import { useDepartments } from '@/composables/useDepartments'
import { useRoles } from '@/composables/useRoles'
import type { Department, Role, User } from '@/models'

export interface UseUsers {
  readonly users: Ref<readonly User[]>
  readonly roles: ComputedRef<readonly Role[]>
  readonly departments: ComputedRef<readonly Department[]>
  readonly loading: ComputedRef<boolean>
  readonly error: ComputedRef<string | null>
  readonly keyword: Ref<string>
  load: () => Promise<void>
  setKeyword: (value: string) => void
  createUser: (body: { username: string; name: string; password: string; role_ids: number[]; department_id?: number | null }) => Promise<void>
  updateUser: (id: number, body: { username: string; name: string; role_ids: number[]; department_id?: number | null }) => Promise<void>
  deleteUser: (id: number) => Promise<void>
  resetUserPassword: (id: number, password: string) => Promise<void>
  createRole: (body: { name: string; description: string; permissions: string[] }) => Promise<void>
  updateRole: (id: number, body: { name: string; description: string; permissions: string[] }) => Promise<void>
  deleteRole: (id: number) => Promise<void>
  createDepartment: (body: { name: string; description?: string; expected_count?: number }) => Promise<void>
  updateDepartment: (id: number, body: { name: string; description?: string; expected_count?: number }) => Promise<void>
  deleteDepartment: (id: number) => Promise<void>
}

export function useUsers(): UseUsers {
  const keyword = ref('')

  const userAsync = useAsync(() => userApi.list({ q: keyword.value || undefined }))
  // 角色 / 部门名单：共享数据源（模块级单例），不再各自拉一份。
  const { roles, loading: rolesLoading, error: rolesError, load: loadRoles } = useRoles()
  const { departments, loading: departmentsLoading, error: departmentsError, load: loadDepartments } = useDepartments()

  const users = computed<readonly User[]>(() => userAsync.data.value?.items ?? [])

  async function load(): Promise<void> {
    await Promise.all([userAsync.run(), loadRoles(), loadDepartments()])
  }

  function setKeyword(value: string): void {
    keyword.value = value
    void userAsync.run()
  }

  async function createUser(body: { username: string; name: string; password: string; role_ids: number[]; department_id?: number | null }): Promise<void> {
    await userApi.create(body)
    await userAsync.run()
  }

  async function updateUser(id: number, body: { username: string; name: string; role_ids: number[]; department_id?: number | null }): Promise<void> {
    await userApi.update(id, body)
    await userAsync.run()
  }

  async function deleteUser(id: number): Promise<void> {
    await userApi.remove(id)
    await userAsync.run()
  }

  async function resetUserPassword(id: number, password: string): Promise<void> {
    await userApi.resetPassword(id, password)
  }

  async function createRole(body: { name: string; description: string; permissions: string[] }): Promise<void> {
    await roleApi.create(body)
    await loadRoles()
  }

  async function updateRole(id: number, body: { name: string; description: string; permissions: string[] }): Promise<void> {
    await roleApi.update(id, body)
    await loadRoles()
  }

  async function deleteRole(id: number): Promise<void> {
    await roleApi.remove(id)
    await loadRoles()
  }

  async function createDepartment(body: { name: string; description?: string }): Promise<void> {
    await departmentApi.create(body)
    await loadDepartments()
  }

  async function updateDepartment(id: number, body: { name: string; description?: string }): Promise<void> {
    await departmentApi.update(id, body)
    await loadDepartments()
  }

  async function deleteDepartment(id: number): Promise<void> {
    await departmentApi.remove(id)
    await loadDepartments()
  }

  return {
    users,
    roles,
    departments,
    loading: computed(() => userAsync.loading.value || rolesLoading.value || departmentsLoading.value),
    error: computed(() => userAsync.error.value || rolesError.value || departmentsError.value),
    keyword,
    load,
    setKeyword,
    createUser,
    updateUser,
    deleteUser,
    resetUserPassword,
    createRole,
    updateRole,
    deleteRole,
    createDepartment,
    updateDepartment,
    deleteDepartment,
  }
}
