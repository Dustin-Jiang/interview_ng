/**
 * useDepartments —— 部门名单（`GET /departments`）的**唯一共享数据源**。
 *
 * 模块级单例（同 `useSystemStatus` 手法）：全站只有一份缓存、一份实现。五个读取方共用它——
 * 用户/角色管理、登录认证页的两类规则目标下拉、候选人页的跨部门归属、志愿编辑弹窗的部门下拉、
 * 系统状态页的录取预览——不再各自 `departmentApi.list()`。
 *
 * **不自作主张拉取**（与 `useSystemStatus` 的「首个访问即拉一次」不同）：部门名单的读取方各有门控
 * （志愿弹窗仅在打开时、候选人页仅在 `candidates.browse_all` 时、预览仅在浏览阶段时），
 * 由它们按自己的门控调用 `load()`，才不会给不需要它的页面平白多发一个请求；
 * 写入方（部门增删改）落库后同样调 `load()` 强制刷新，其余端下次读取即最新。
 */
import { computed, type ComputedRef, type Ref } from 'vue'

import { departmentApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { Department } from '@/models'

export interface UseDepartments {
  /** 部门名单（未加载为 []）。 */
  readonly departments: ComputedRef<readonly Department[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  /** 重新拉取（整份替换；失败收口在 `error`，不抛）。 */
  load: () => Promise<void>
}

const list = useAsync(() => departmentApi.list())
const departments = computed<readonly Department[]>(() => list.data.value?.items ?? [])

async function load(): Promise<void> {
  await list.run()
}

export function useDepartments(): UseDepartments {
  return { departments, loading: list.loading, error: list.error, load }
}
