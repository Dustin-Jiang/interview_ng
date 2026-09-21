/**
 * useRosterSelection —— 名册「选中 + 排序切换 + 键盘导航」组合式函数。
 * 收编「候选人查看」与「捡漏竞拍」两页同构的选择态：
 *  - 选中项随筛选结果自动回退（选中项被筛掉时落到首位可见项）；
 *  - 条目路由参数 `/…/:candidateId` 解析后待列表就绪落地（在名册内则选中并进入详情）；
 *  - 上一个 / 下一个（←/→ 全局键，输入控件聚焦时不拦截）与移动端两段式状态。
 * 筛选条件本身（关键词 / 状态）由调用方维护，本函数只消费「筛选后的列表」。
 */
import { computed, onBeforeUnmount, onMounted, ref, watch, type ComputedRef, type Ref } from 'vue'

import { useMasterDetail } from '@/composables/useMasterDetail'

export interface RosterSelectionOptions<T extends { id: number }> {
  /** 当前可见（筛选后）候选列表。 */
  items: () => readonly T[]
  /** 未筛选中候选集查询：用于深链落地（链接指向的候选人可能被当前筛选隐藏）。 */
  lookup?: (id: number) => T | null
  /** 数据是否已就绪：未就绪时空列表不消费深链待选项（避免被首帧空列表提前落地）。 */
  ready?: () => boolean
  /** 全局键的额外快捷键；返回 true 表示已消费本次按键。 */
  extraKeys?: (e: KeyboardEvent) => boolean
}

export interface UseRosterSelection<T extends { id: number }> {
  readonly selectedId: Ref<number | null>
  readonly selected: ComputedRef<T | null>
  /** 选中并进入详情（点击名册项）。 */
  select: (item: T) => void
  /** 仅移动选中高亮（列表内方向键），不切换移动端视图。 */
  highlight: (item: T) => void
  readonly canPrev: ComputedRef<boolean>
  readonly canNext: ComputedRef<boolean>
  goPrev: () => void
  goNext: () => void
  /** 移动端两段式状态。 */
  readonly showDetail: Ref<boolean>
  closeDetail: () => void
  /** 从路由参数恢复待选条目（列表就绪后自动落地）。 */
  presetSelection: (raw: unknown) => void
}

/** 全局方向键忽略输入控件聚焦（搜索框、数字输入等）。 */
function isEditableTarget(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el) return false
  return (
    el.tagName === 'INPUT' ||
    el.tagName === 'TEXTAREA' ||
    el.tagName === 'SELECT' ||
    el.isContentEditable
  )
}

export function useRosterSelection<T extends { id: number }>(
  opts: RosterSelectionOptions<T>,
): UseRosterSelection<T> {
  const selectedId = ref<number | null>(null)
  const { showDetail, openDetail, closeDetail } = useMasterDetail()

  /** 深链待选 id：列表就绪后若在名册内则选中。 */
  let pendingId: number | undefined

  const selectedIndex = computed(() => opts.items().findIndex((c) => c.id === selectedId.value))
  const selected = computed(() => {
    const id = selectedId.value
    // 选中项被当前筛选隐藏时，仍可经未筛选集还原（深链指向的候选人）。
    return opts.items().find((c) => c.id === id) ?? (id == null || !opts.lookup ? null : opts.lookup(id))
  })
  const canPrev = computed(() => selectedIndex.value > 0)
  const canNext = computed(
    () => selectedIndex.value !== -1 && selectedIndex.value < opts.items().length - 1,
  )

  function highlight(item: T): void {
    selectedId.value = item.id
  }

  function select(item: T): void {
    highlight(item)
    openDetail()
  }

  function goPrev(): void {
    const list = opts.items()
    if (canPrev.value) select(list[selectedIndex.value - 1])
  }

  function goNext(): void {
    const list = opts.items()
    if (canNext.value) select(list[selectedIndex.value + 1])
  }

  function presetSelection(raw: unknown): void {
    if (typeof raw === 'string' && /^\d+$/.test(raw)) {
      pendingId = Number(raw)
      openDetail()
    }
  }

  // 列表变化：先落地深链待选项，其后保证选中项始终可见（被筛掉则回退首位）。
  watch(
    opts.items,
    (list) => {
      if (pendingId != null) {
        const id = pendingId
        const hit = list.find((c) => c.id === id) ?? opts.lookup?.(id) ?? null
        if (hit) {
          pendingId = undefined
          highlight(hit)
          return
        }
        // 数据未就绪（如首帧空列表）时保留待选项，待数据到达后再落地。
        if (opts.ready && !opts.ready()) return
        pendingId = undefined
      }
      if (list.length === 0) {
        selectedId.value = null
        return
      }
      if (!list.some((c) => c.id === selectedId.value)) highlight(list[0])
    },
    { immediate: true },
  )

  // 键盘 ←/→ 切换候选人；输入控件聚焦时不拦截。
  function onGlobalKeydown(e: KeyboardEvent) {
    if (isEditableTarget(e.target)) return
    if (opts.extraKeys?.(e)) return
    if (e.key === 'ArrowLeft') {
      goPrev()
      return
    }
    if (e.key === 'ArrowRight') goNext()
  }

  onMounted(() => window.addEventListener('keydown', onGlobalKeydown))
  onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKeydown))

  return {
    selectedId,
    selected,
    select,
    highlight,
    canPrev,
    canNext,
    goPrev,
    goNext,
    showDetail,
    closeDetail,
    presetSelection,
  }
}
