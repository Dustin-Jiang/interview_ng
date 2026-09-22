<script setup lang="ts">
import type { Cell, ColumnDef } from '@tanstack/vue-table'
import { FlexRender, useTable } from '@tanstack/vue-table'
import DataTablePagination from './data-table-pagination.vue'
import { features, type DataTableFeatures } from './features'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '.'

/**
 * 通用数据表：≥md 渲染常规表格，<md 把每行渲染成一张卡片。
 * 手机上宽表（设置页 6~15 列）只能逐列横向滚动，扫一眼就要来回滑，等于丢失上下文；
 * 卡片模式把同一行的单元格按「标签 + 值」纵向罗列，一屏内读完一条记录。
 * 列的约定（全仓库统一）：id 为 `actions` 的列是操作区，卡片里整行铺在底部；
 * 有字符串列头的列作字段标签；无列头的展示列并入卡片标题行右侧（不丢信息）。
 */
const props = defineProps<{
  columns: ColumnDef<DataTableFeatures, any>[]
  data: readonly any[]
}>()

const table = useTable({
  features,
  get data() {
    return props.data as any[]
  },
  get columns() {
    return props.columns
  },
})

/** 列头文本（仅字符串列头可作字段标签）。 */
function labelOf(cell: Cell<DataTableFeatures, any>): string {
  const header = cell.column.columnDef.header
  return typeof header === 'string' ? header.trim() : ''
}

function isActionCell(cell: Cell<DataTableFeatures, any>): boolean {
  return cell.column.id === 'actions'
}

/** 首个单元格（学号 / 姓名 / 名称这类标识列）作卡片标题。 */
function titleCell(cells: Cell<DataTableFeatures, any>[]): Cell<DataTableFeatures, any> | undefined {
  return cells[0]
}

/** 无列头、非操作列的单元格（如结果徽章）：并到标题行右侧。 */
function extraCells(cells: Cell<DataTableFeatures, any>[]): Cell<DataTableFeatures, any>[] {
  return cells.slice(1).filter((c) => !isActionCell(c) && !labelOf(c))
}

/** 「标签 + 值」字段行。 */
function fieldCells(cells: Cell<DataTableFeatures, any>[]): Cell<DataTableFeatures, any>[] {
  return cells.slice(1).filter((c) => !isActionCell(c) && !!labelOf(c))
}

function actionCells(cells: Cell<DataTableFeatures, any>[]): Cell<DataTableFeatures, any>[] {
  return cells.slice(1).filter(isActionCell)
}
</script>

<template>
  <div class="space-y-3 text-sm">
    <!-- ≥md：常规表格 -->
    <div class="hidden rounded-md border md:block">
      <Table>
        <TableHeader>
          <TableRow v-for="headerGroup in table.getHeaderGroups()" :key="headerGroup.id">
            <TableHead v-for="header in headerGroup.headers" :key="header.id">
              <FlexRender v-if="!header.isPlaceholder" :header="header" />
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <template v-if="table.getRowModel().rows?.length">
            <TableRow v-for="row in table.getRowModel().rows" :key="row.id" class="h-12">
              <TableCell v-for="cell in row.getVisibleCells()" :key="cell.id">
                <FlexRender :cell="cell" />
              </TableCell>
            </TableRow>
          </template>
          <template v-else>
            <TableRow>
              <TableCell :colspan="columns.length" class="h-24 text-center text-muted-foreground">
                暂无数据
              </TableCell>
            </TableRow>
          </template>
        </TableBody>
      </Table>
    </div>

    <!-- <md：一记录一卡片（[&_*]:whitespace-normal 抵消表格单元格的 nowrap，
         否则长值会把卡片顶宽、又被外层裁掉） -->
    <ul v-if="table.getRowModel().rows?.length" class="space-y-2 md:hidden">
      <li v-for="row in table.getRowModel().rows" :key="row.id" class="rounded-md border p-3">
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0 text-base font-medium [&_*]:min-w-0 [&_*]:whitespace-normal">
            <FlexRender v-if="titleCell(row.getVisibleCells())" :cell="titleCell(row.getVisibleCells())!" />
          </div>
          <div
            v-for="cell in extraCells(row.getVisibleCells())"
            :key="cell.id"
            class="shrink-0 [&_*]:whitespace-normal"
          >
            <FlexRender :cell="cell" />
          </div>
        </div>

        <dl v-if="fieldCells(row.getVisibleCells()).length" class="mt-2 space-y-1.5">
          <div
            v-for="cell in fieldCells(row.getVisibleCells())"
            :key="cell.id"
            class="flex items-start gap-3"
          >
            <dt class="w-20 shrink-0 pt-0.5 text-xs text-muted-foreground">{{ labelOf(cell) }}</dt>
            <dd class="min-w-0 flex-1 break-words [&_*]:min-w-0 [&_*]:whitespace-normal">
              <FlexRender :cell="cell" />
            </dd>
          </div>
        </dl>

        <div v-if="actionCells(row.getVisibleCells()).length" class="mt-3 flex flex-wrap gap-2">
          <FlexRender v-for="cell in actionCells(row.getVisibleCells())" :key="cell.id" :cell="cell" />
        </div>
      </li>
    </ul>
    <p v-else class="rounded-md border p-6 text-center text-muted-foreground md:hidden">暂无数据</p>

    <DataTablePagination :table="table" />
  </div>
</template>
