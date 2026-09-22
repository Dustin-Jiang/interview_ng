<!--
  ImportPreviewTable —— 导入实时预览：分页表格直接承载全部映射结果（无「只看前 N 行」切换，
  分页控件负责浏览），每行按目标字段展开（学号 / 姓名 / 个人简介）+ 校验状态。
  表格不嵌套 Card：标题与统计条分别走 DataTableSection 的标题与工具条插槽。
-->
<script setup lang="ts">
import { computed, h } from 'vue'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import DataTableSection from '@/components/app/DataTableSection.vue'
import { Badge } from '@/components/ui/badge'
import type { DataTableFeatures } from '@/components/ui/table/features'
import type { ImportOutcome } from '@/domain/import'

const props = defineProps<{
  outcome: ImportOutcome
}>()

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

const rows = computed<PreviewRow[]>(() =>
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
 * 除个人简介（可换行、给最小宽度以保持可读）外，各列一律 nowrap —— 内容不折行，
 * 放不下时由表格容器横向滚动。
 */
function textCell(value: string, classNames: string) {
  return value ? h('div', { class: classNames }, value) : h('span', { class: 'text-muted-foreground' }, '-')
}

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
    cell: ({ getValue }) => textCell(String(getValue() ?? ''), 'min-w-56 break-words text-muted-foreground'),
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
  <DataTableSection
    title="实时预览"
    :loading="false"
    :items="rows"
    :columns="columns"
    :data="rows"
    empty-text="尚无映射结果"
  >
    <template #toolbar>
      <div class="flex flex-wrap items-center gap-2">
        <Badge variant="outline">总 {{ props.outcome.stats.total }}</Badge>
        <Badge variant="secondary">新建 {{ props.outcome.stats.create }}</Badge>
        <Badge variant="outline">更新 {{ props.outcome.stats.update }}</Badge>
        <Badge :variant="props.outcome.stats.failed ? 'destructive' : 'outline'">
          失败 {{ props.outcome.stats.failed }}
        </Badge>
      </div>
    </template>
  </DataTableSection>
</template>
