/**
 * useUsers —— 面试官与角色、部门管理组合式函数（函数式 ViewModel，Q28：角色管理门控于 users.manage）。
 */
import { computed, ref, type Ref } from 'vue'
import { departmentApi, roleApi, userApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { Department, Role, User } from '@/models'

export interface UseUsers {
  readonly users: Ref<readonly User[]>
  readonly roles: Ref<readonly Role[]>
  readonly departments: Ref<readonly Department[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
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
  const roleAsync = useAsync(() => roleApi.list())
  const departmentAsync = useAsync(() => departmentApi.list())

  const users = computed<readonly User[]>(() => userAsync.data.value?.items ?? [])
  const roles = computed<readonly Role[]>(() => roleAsync.data.value?.items ?? [])
  const departments = computed<readonly Department[]>(() => departmentAsync.data.value?.items ?? [])

  async function load(): Promise<void> {
    await Promise.all([userAsync.run(), roleAsync.run(), departmentAsync.run()])
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
    await roleAsync.run()
  }

  async function updateRole(id: number, body: { name: string; description: string; permissions: string[] }): Promise<void> {
    await roleApi.update(id, body)
    await roleAsync.run()
  }

  async function deleteRole(id: number): Promise<void> {
    await roleApi.remove(id)
    await roleAsync.run()
  }

  async function createDepartment(body: { name: string; description?: string }): Promise<void> {
    await departmentApi.create(body)
    await departmentAsync.run()
  }

  async function updateDepartment(id: number, body: { name: string; description?: string }): Promise<void> {
    await departmentApi.update(id, body)
    await departmentAsync.run()
  }

  async function deleteDepartment(id: number): Promise<void> {
    await departmentApi.remove(id)
    await departmentAsync.run()
  }

  return {
    users,
    roles,
    departments,
    loading: computed(() => userAsync.loading.value || roleAsync.loading.value || departmentAsync.loading.value),
    error: computed(() => userAsync.error.value || roleAsync.error.value || departmentAsync.error.value),
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
