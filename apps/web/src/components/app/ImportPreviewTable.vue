<!--
  ImportPreviewTable —— 导入实时预览：分页表格直接承载全部映射结果（无「只看前 N 行」切换，
  分页控件负责浏览），每行按目标字段展开（学号 / 姓名 / 个人简介 / 更新时间）+ 校验状态；
  统计标签（总 / 新建 / 更新 / 跳过 / 失败）同时是筛选入口。
  表格不嵌套 Card：标题与统计条分别走 DataTableSection 的标题与工具条插槽。
-->
<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import ClampText from '@/components/app/ClampText.vue'
import DataTableSection from '@/components/app/DataTableSection.vue'
import { Badge } from '@/components/ui/badge'
import { textCell } from '@/components/ui/table/cells'
import type { DataTableFeatures } from '@/components/ui/table/features'
import type { ImportOutcome } from '@/domain/import'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

const props = defineProps<{
  outcome: ImportOutcome
}>()

/** 表格筛选维度：总 / 新建 / 更新 / 跳过 / 失败（点标签切换，再点一次回到「总」）。 */
type FilterKey = 'all' | 'create' | 'update' | 'skip' | 'error'
const filter = ref<FilterKey>('all')

/** 筛选标签（计数取整体结果，标签本身即筛选入口）。 */
const filterOptions = computed(() => {
  const stats = props.outcome.stats
  return [
    { key: 'all', label: '总', count: stats.total, variant: 'outline' },
    { key: 'create', label: '新建', count: stats.create, variant: 'secondary' },
    { key: 'update', label: '更新', count: stats.update, variant: 'outline' },
    { key: 'skip', label: '跳过', count: stats.skip, variant: 'outline' },
    { key: 'error', label: '失败', count: stats.failed, variant: stats.failed ? 'destructive' : 'outline' },
  ] as const
})

function toggleFilter(key: FilterKey): void {
  filter.value = filter.value === key && key !== 'all' ? 'all' : key
}

interface PreviewRow {
  line: number
  studentNo: string
  name: string
  firstChoice: string
  secondChoice: string
  /** 「是否接受调剂」展示文本：接受 / 不接受；未映射或空单元格为 `-`。 */
  acceptAdjust: string
  phone: string
  qq: string
  email: string
  profile: string
  /** 源表给出的该行更新时间（RFC3339）；未映射 / 空单元格为 `-`。 */
  updatedAt: string
  status: 'create' | 'update' | 'skip' | 'error'
  /** 状态标签：新建 / 更新 / 覆盖第 N 行 / 已跳过（含库中更新时间）/ 不合法。 */
  label: string
  errors: string[]
}

const allRows = computed<PreviewRow[]>(() =>
  props.outcome.mapped.map((row) => ({
    line: row.line,
    studentNo: row.studentNo,
    name: row.name,
    firstChoice: row.firstChoice,
    secondChoice: row.secondChoice,
    acceptAdjust: row.acceptAdjustProvided ? (row.acceptAdjust ? '接受' : '不接受') : '',
    phone: row.phone,
    qq: row.qq,
    email: row.email,
    profile: row.profile,
    updatedAt: row.updatedAt,
    status: row.status,
    label:
      row.status === 'create'
        ? '新建'
        : row.status === 'update'
          ? row.overridesLine
            ? `覆盖第 ${row.overridesLine} 行`
            : '更新'
          : row.status === 'skip'
            ? `已跳过（库中更新于 ${formatDateTime(row.storedUpdatedAt)}）`
            : '不合法',
    errors: row.errors,
  })),
)

/**
 * 当前筛选下的行（分页作用于筛选后的集合）。
 * 空态判定仍用未筛选的全集（DataTableSection 的 items 语义）：筛出 0 行时保留表头、
 * 统计标签与分页条（否则筛选入口会随表格一起消失，无法切回「总」）。
 */
const rows = computed(() =>
  filter.value === 'all' ? allRows.value : allRows.value.filter((row) => row.status === filter.value),
)

const columnHelper = createColumnHelper<DataTableFeatures, PreviewRow>()
const columns: ColumnDef<DataTableFeatures, PreviewRow>[] = columnHelper.columns([
  columnHelper.accessor('line', {
    header: '行号',
    enableSorting: false,
    cell: ({ getValue }) =>
      h('div', { class: 'whitespace-nowrap tabular-nums text-muted-foreground' }, String(getValue())),
  }),
  columnHelper.accessor('studentNo', {
    header: '学号',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap tabular-nums'),
  }),
  columnHelper.accessor('name', {
    header: '姓名',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap'),
  }),
  columnHelper.accessor('firstChoice', {
    header: '第一志愿',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap'),
  }),
  columnHelper.accessor('secondChoice', {
    header: '第二志愿',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap'),
  }),
  columnHelper.accessor('acceptAdjust', {
    header: '接受调剂',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap'),
  }),
  columnHelper.accessor('phone', {
    header: '手机号',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap tabular-nums'),
  }),
  columnHelper.accessor('qq', {
    header: 'QQ号',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap tabular-nums'),
  }),
  columnHelper.accessor('email', {
    header: '邮箱',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap tabular-nums'),
  }),
  columnHelper.accessor('profile', {
    header: '个人简介',
    enableSorting: false,
    cell: ({ getValue }) => {
      const value = String(getValue() ?? '')
      if (!value) return h('span', { class: 'text-muted-foreground' }, '-')
      // 最多 3 行 + 省略号，溢出时点击用 Popover 看全文。
      // min-w-56 只在 ≥md 生效：手机卡片模式里它是绝对下限，会把卡片撑宽（横向滚动随之出现）。
      return h(ClampText, { text: value, lines: 3, class: 'text-muted-foreground md:min-w-56' })
    },
  }),
  columnHelper.accessor('updatedAt', {
    header: '更新时间',
    enableSorting: false,
    cell: ({ getValue }) => textCell(formatDateTime(String(getValue() ?? '')), 'whitespace-nowrap tabular-nums'),
  }),
  columnHelper.display({
    id: 'status',
    header: '校验状态',
    enableHiding: false,
    // 不再对整格强制 nowrap（徽章自身已 nowrap）：手机上卡片模式里，nowrap 的错误原因会横向溢出卡片。
    cell: ({ row }) =>
      h('div', { class: 'space-y-1 break-words' }, [
        h(
          Badge,
          {
            variant:
              row.original.status === 'error'
                ? 'destructive'
                : row.original.status === 'skip'
                  ? 'outline'
                  : 'secondary',
            class: 'text-sm',
          },
          () => row.original.label,
        ),
        ...row.original.errors.map((message) => h('p', { class: 'text-destructive' }, message)),
      ]),
  }),
])
</script>

<template>
  <!-- 表头与数据列一致地不换行（个人简介列的数据仍可换行）。 -->
  <DataTableSection
    title="预览"
    nowrap-headers
    :loading="false"
    :items="allRows"
    :columns="columns"
    :data="rows"
    empty-text="尚无映射结果"
  >
    <template #toolbar>
      <!-- 标签即筛选：点击只看该类行，再点一次回到「总」（aria-pressed 表达选中态）。
           触屏（≤lg）把每个标签的点击区抬到 44px；容器已换行，窄屏下标签逐行排布不横向溢出。 -->
      <div class="flex flex-wrap items-center gap-2">
        <button
          v-for="option in filterOptions"
          :key="option.key"
          type="button"
          class="rounded-md transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-lg:inline-flex max-lg:min-h-11 max-lg:items-center"
          :aria-pressed="filter === option.key"
          @click="toggleFilter(option.key)"
        >
          <Badge :variant="option.variant" :class="cn(filter === option.key && 'ring-2 ring-ring')">
            {{ option.label }} {{ option.count }}
          </Badge>
        </button>
      </div>
    </template>
  </DataTableSection>
</template>
