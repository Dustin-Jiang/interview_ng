/**
 * useRoles —— 角色名单（`GET /roles`）的**唯一共享数据源**。
 *
 * 模块级单例（同 `useDepartments` 手法）：用户/角色管理与登录认证页的角色规则目标下拉
 * 共用同一份缓存，不再各自 `roleApi.list()`。
 *
 * **不自作主张拉取**：由读取方按自己的门控调用 `load()`；
 * 写入方（角色增删改）落库后同样调 `load()` 强制刷新，其余端下次读取即最新。
 */
import { computed, type ComputedRef, type Ref } from 'vue'

import { roleApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { Role } from '@/models'

export interface UseRoles {
  /** 角色名单（未加载为 []）。 */
  readonly roles: ComputedRef<readonly Role[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  /** 重新拉取（整份替换；失败收口在 `error`，不抛）。 */
  load: () => Promise<void>
}

const list = useAsync(() => roleApi.list())
const roles = computed<readonly Role[]>(() => list.data.value?.items ?? [])

async function load(): Promise<void> {
  await list.run()
}

export function useRoles(): UseRoles {
  return { roles, loading: list.loading, error: list.error, load }
}
