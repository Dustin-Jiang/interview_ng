<script setup lang="ts">
import type { ColumnDef } from '@tanstack/vue-table'
import { FlexRender, useTable } from '@tanstack/vue-table'
import DataTablePagination from './data-table-pagination.vue'
import { features, type DataTableFeatures } from './features'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '.'

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
</script>

<template>
  <div class="space-y-3">
    <div class="rounded-md border">
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
            <TableRow
              v-for="row in table.getRowModel().rows"
              :key="row.id"
              class="h-12"
            >
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
    <DataTablePagination :table="table" />
  </div>
</template>
