/**
 * TanStack Table v9 特性配置（DataTable 共享）。
 * 只注册本项目用到的特性/排序/过滤函数，未被注册的能力会被 tree-shake 掉。
 */
import {
  columnFilteringFeature,
  columnVisibilityFeature,
  createFilteredRowModel,
  createPaginatedRowModel,
  createSortedRowModel,
  filterFn_includesString,
  rowPaginationFeature,
  rowSortingFeature,
  sortFn_alphanumeric,
  sortFn_text,
  tableFeatures as featuresFactory,
} from '@tanstack/vue-table'

/** 表特性：排序 + 列过滤 + 列显隐 + 分页。 */
export const features = featuresFactory({
  columnFilteringFeature,
  columnVisibilityFeature,
  rowPaginationFeature,
  rowSortingFeature,
  filteredRowModel: createFilteredRowModel(),
  paginatedRowModel: createPaginatedRowModel(),
  sortedRowModel: createSortedRowModel(),
  filterFns: { includesString: filterFn_includesString },
  sortFns: { alphanumeric: sortFn_alphanumeric, text: sortFn_text },
})

/** 列定义 / 表类型的第一泛型参数。 */
export type DataTableFeatures = typeof features
