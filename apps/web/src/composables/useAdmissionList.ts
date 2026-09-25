/**
 * useAdmissionList —— 录取决定列表（`GET /admissions`，候选人 × 部门）的**唯一共享数据源**。
 *
 * 模块级单例（同 `useDepartments` 手法）：候选人查看页（本部门控件 + 跨部门归属）、
 * 部门管理页的录取三档统计、系统状态页的录取预览三处共用同一份缓存与同一份实现，
 * 不再各自 `admissionApi.list()`。
 *
 * 录取决定**没有看板事件**（改动只发生在应用内），因此新鲜度只由两件事决定：
 *  - 各读取方按自己的门控调用 `load()`（候选人页在控件可见时、统计与预览在挂载/手动刷新时）；
 *  - 写入方 `useAdmissions#switchAdmission` 落库后立刻 `load()` 强制刷新。
 * 服务端在进入结算阶段时会按出价批量同步录取档，前端无从感知——各读取方下次 `load()` 即最新，
 * 与合并成共享源之前的刷新时机完全一致。
 */
import { computed, type ComputedRef, type Ref } from 'vue'

import { admissionApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import type { CandidateAdmission } from '@/models'

export interface UseAdmissionList {
  /** 全部（候选人 × 部门）录取决定（未加载为 []）。 */
  readonly admissions: ComputedRef<readonly CandidateAdmission[]>
  readonly loading: Ref<boolean>
  readonly error: Ref<string | null>
  /** 重新拉取（整份替换；失败收口在 `error`，不抛）。 */
  load: () => Promise<void>
}

const list = useAsync(() => admissionApi.list())
const admissions = computed<readonly CandidateAdmission[]>(() => list.data.value?.items ?? [])

async function load(): Promise<void> {
  await list.run()
}

export function useAdmissionList(): UseAdmissionList {
  return { admissions, loading: list.loading, error: list.error, load }
}
