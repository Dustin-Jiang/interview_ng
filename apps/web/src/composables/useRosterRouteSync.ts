/**
 * useRosterRouteSync —— 名册页面与「集合 / 条目」路由的双向同步。
 * 名册本身不感知路由：选中条目 id 走路径参数（`/candidates/:candidateId`、
 * `/leftover/candidates/:candidateId`），筛选/搜索等视图态仍在 query；
 * 同时处理挂载恢复、历史前进/后退与外部跳转的选中态同步。
 */
import { onMounted, watch, type Ref } from 'vue'
import { useRoute } from 'vue-router'

import { useUrlSync } from '@/composables/useUrlSync'

export interface RosterRouteSync<T extends { id: number }> {
  /** 集合路由名（无选中条目时写回）。 */
  listRoute: string
  /** 条目路由名（有选中条目时写回，参数名为 `candidateId`）。 */
  itemRoute: string
  /** 名册选中态（由 useRosterSelection 提供）。 */
  selection: {
    selectedId: Ref<number | null>
    select: (item: T) => void
    presetSelection: (raw: unknown) => void
  }
  /** 未筛选条目集查询（深链指向的条目可能被当前筛选隐藏）。 */
  lookup: (id: number) => T | null
  /** 视图态 query（筛选/搜索），空值由调用方省略。 */
  query: () => Record<string, string>
  /** 挂载时的额外恢复（如从 query 还原筛选条件）。 */
  onMount?: () => void
}

/** 路径参数 → 数字 id（非数字或缺失返回 null）。 */
function paramId(raw: unknown): number | null {
  return typeof raw === 'string' && /^\d+$/.test(raw) ? Number(raw) : null
}

export function useRosterRouteSync<T extends { id: number }>(opts: RosterRouteSync<T>): void {
  const route = useRoute()
  const { selectedId } = opts.selection

  onMounted(() => {
    opts.onMount?.()
    opts.selection.presetSelection(route.params.candidateId)
  })

  // 历史前进/后退或外部跳转改变条目参数时同步选中态（自身写回因 id 相等而跳过）。
  watch(
    () => route.params.candidateId,
    (raw) => {
      const id = paramId(raw)
      if (id == null || id === selectedId.value) return
      const hit = opts.lookup(id)
      if (hit) opts.selection.select(hit)
      else opts.selection.presetSelection(id)
    },
  )

  useUrlSync(
    () =>
      selectedId.value
        ? { name: opts.itemRoute, params: { candidateId: String(selectedId.value) }, query: opts.query() }
        : { name: opts.listRoute, query: opts.query() },
    () => [selectedId.value, opts.query()],
  )
}
