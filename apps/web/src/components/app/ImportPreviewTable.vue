<!--
  ImportPreviewTable —— 导入实时预览：分页表格直接承载全部映射结果（无「只看前 N 行」切换，
  分页控件负责浏览），每行按目标字段展开（学号 / 姓名 / 个人简介）+ 校验状态；
  统计标签（总 / 新建 / 更新 / 失败）同时是筛选入口。
  表格不嵌套 Card：标题与统计条分别走 DataTableSection 的标题与工具条插槽。
-->
<script setup lang="ts">
import { computed, h, ref } from 'vue'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import DataTableSection from '@/components/app/DataTableSection.vue'
import { Badge } from '@/components/ui/badge'
import type { DataTableFeatures } from '@/components/ui/table/features'
import type { ImportOutcome } from '@/domain/import'
import { cn } from '@/lib/utils'

const props = defineProps<{
  outcome: ImportOutcome
}>()

/** 表格筛选维度：总 / 新建 / 更新 / 失败（点标签切换，再点一次回到「总」）。 */
type FilterKey = 'all' | 'create' | 'update' | 'error'
const filter = ref<FilterKey>('all')

/** 筛选标签（计数取整体结果，标签本身即筛选入口）。 */
const filterOptions = computed(() => {
  const stats = props.outcome.stats
  return [
    { key: 'all', label: '总', count: stats.total, variant: 'outline' },
    { key: 'create', label: '新建', count: stats.create, variant: 'secondary' },
    { key: 'update', label: '更新', count: stats.update, variant: 'outline' },
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
  profile: string
  status: 'create' | 'update' | 'error'
  /** 状态标签：新建 / 更新 / 覆盖第 N 行 / 不合法。 */
  label: string
  errors: string[]
}

const allRows = computed<PreviewRow[]>(() =>
  props.outcome.mapped.map((row) => ({
    line: row.line,
    studentNo: row.studentNo,
    name: row.name,
    profile: row.profile,
    status: row.status,
    label:
      row.status === 'create'
        ? '新建'
        : row.status === 'update'
          ? row.overridesLine
            ? `覆盖第 ${row.overridesLine} 行`
            : '更新'
          : '不合法',
    errors: row.errors,
  })),
)

/**
 * 单元格文本：空值统一以 `-` 占位（不区分「映射为空」与「未映射」）。
 * 除个人简介（whitespace-pre-line：**保留单元格内的换行**、过长再折行，并给最小宽度以保持可读）
 * 外，各列一律 nowrap —— 内容不折行，放不下时由表格容器横向滚动。
 */
function textCell(value: string, classNames: string) {
  return value ? h('div', { class: classNames }, value) : h('span', { class: 'text-muted-foreground' }, '-')
}

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
      h('div', { class: 'whitespace-nowrap font-mono text-xs text-muted-foreground' }, String(getValue())),
  }),
  columnHelper.accessor('studentNo', {
    header: '学号',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap font-mono text-xs'),
  }),
  columnHelper.accessor('name', {
    header: '姓名',
    enableSorting: false,
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'whitespace-nowrap'),
  }),
  columnHelper.accessor('profile', {
    header: '个人简介',
    enableSorting: false,
    cell: ({ getValue }) =>
      textCell(String(getValue() ?? ''), 'min-w-56 whitespace-pre-line break-words text-muted-foreground'),
  }),
  columnHelper.display({
    id: 'status',
    header: '校验状态',
    enableHiding: false,
    cell: ({ row }) =>
      h('div', { class: 'space-y-1 whitespace-nowrap' }, [
        h(
          Badge,
          { variant: row.original.status === 'error' ? 'destructive' : 'secondary' },
          () => row.original.label,
        ),
        ...row.original.errors.map((message) => h('p', { class: 'text-xs text-destructive' }, message)),
      ]),
  }),
])
</script>

<template>
  <!-- [&_th]：表头与数据列一致地不换行（个人简介列的数据仍可换行）。 -->
  <div class="[&_th]:whitespace-nowrap">
    <DataTableSection
      title="实时预览"
      :loading="false"
      :items="allRows"
      :columns="columns"
      :data="rows"
      empty-text="尚无映射结果"
    >
      <template #toolbar>
        <!-- 标签即筛选：点击只看该类行，再点一次回到「总」（aria-pressed 表达选中态）。 -->
        <div class="flex flex-wrap items-center gap-2">
          <button
            v-for="option in filterOptions"
            :key="option.key"
            type="button"
            class="rounded-md transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
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
  </div>
</template>
