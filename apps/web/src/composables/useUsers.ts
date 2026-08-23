/**
 * useUsers —— 面试官与角色管理组合式函数（函数式 ViewModel，Q28：角色管理门控于 users.manage）。
 */
import { computed, ref, type Ref } from 'vue'
import { roleApi, userApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { Role, User } from '@/models'

export interface UseUsers {
  readonly users: Ref<readonly User[]>
  readonly roles: Ref<readonly Role[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  readonly keyword: Ref<string>
  load: () => Promise<void>
  setKeyword: (value: string) => void
  createUser: (body: { username: string; name: string; password: string; role_ids: number[] }) => Promise<void>
  updateUser: (id: number, body: { name: string; role_ids: number[] }) => Promise<void>
  deleteUser: (id: number) => Promise<void>
  resetUserPassword: (id: number, password: string) => Promise<void>
  createRole: (body: { name: string; description: string; permissions: string[] }) => Promise<void>
  updateRole: (id: number, body: { name: string; description: string; permissions: string[] }) => Promise<void>
  deleteRole: (id: number) => Promise<void>
}

export function useUsers(): UseUsers {
  const keyword = ref('')

  const userAsync = useAsync(() => userApi.list({ q: keyword.value || undefined }))
  const roleAsync = useAsync(() => roleApi.list())

  const users = computed<readonly User[]>(() => userAsync.data.value?.items ?? [])
  const roles = computed<readonly Role[]>(() => roleAsync.data.value?.items ?? [])

  async function load(): Promise<void> {
    await Promise.all([userAsync.run(), roleAsync.run()])
  }

  function setKeyword(value: string): void {
    keyword.value = value
    void userAsync.run()
  }

  async function createUser(body: { username: string; name: string; password: string; role_ids: number[] }): Promise<void> {
    await userApi.create(body)
    await userAsync.run()
  }

  async function updateUser(id: number, body: { name: string; role_ids: number[] }): Promise<void> {
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

  return {
    users,
    roles,
    loading: computed(() => userAsync.loading.value || roleAsync.loading.value),
    error: computed(() => userAsync.error.value || roleAsync.error.value),
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
  }
}
