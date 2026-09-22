<script setup lang="ts">
import type { VueTable } from '@tanstack/vue-table'
import { computed } from 'vue'
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-vue-next'
import type { DataTableFeatures } from './features'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const props = defineProps<{ table: VueTable<DataTableFeatures, any> }>()

// 原子读需在响应式作用域（computed）内，模板中直接调用亦可。
const pagination = computed(() => props.table.atoms.pagination.get())

// 空表（筛选后 0 行）总页数为 0，按 1 页展示，避免出现「第 1 / 0 页」。
const pageCount = computed(() => Math.max(props.table.getPageCount(), 1))
const currentPage = computed(() => Math.min(pagination.value.pageIndex + 1, pageCount.value))
</script>

<template>
  <!-- 分页条：页码/每页条数 + 首页/上一页/下一页/末页（图标按钮带可访问名）。
       手机上允许换行并抬高按钮，避免「固定宽 + 不换行」把整页顶出横向滚动。 -->
  <div class="flex flex-wrap items-center justify-between gap-2 px-2">
    <div class="text-sm text-muted-foreground">
      共 {{ table.getFilteredRowModel().rows.length }} 条
    </div>
    <div class="flex flex-wrap items-center gap-1.5 sm:gap-2">
      <Select
        :model-value="`${pagination.pageSize}`"
        @update:model-value="(v) => table.setPageSize(Number(v))"
      >
        <SelectTrigger class="h-8 w-[70px] max-lg:w-20">
          <SelectValue />
        </SelectTrigger>
        <SelectContent side="top">
          <SelectItem v-for="size in [10, 20, 30, 50]" :key="size" :value="`${size}`">
            {{ size }}
          </SelectItem>
        </SelectContent>
      </Select>
      <span class="whitespace-nowrap text-sm font-medium">
        第 {{ currentPage }} / {{ pageCount }} 页
      </span>
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        aria-label="首页"
        :disabled="!table.getCanPreviousPage()"
        @click="table.setPageIndex(0)"
      >
        <ChevronsLeft class="h-4 w-4" aria-hidden="true" />
      </Button>
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        aria-label="上一页"
        :disabled="!table.getCanPreviousPage()"
        @click="table.previousPage()"
      >
        <ChevronLeft class="h-4 w-4" aria-hidden="true" />
      </Button>
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        aria-label="下一页"
        :disabled="!table.getCanNextPage()"
        @click="table.nextPage()"
      >
        <ChevronRight class="h-4 w-4" aria-hidden="true" />
      </Button>
      <Button
        variant="outline"
        size="icon"
        class="h-8 w-8"
        aria-label="末页"
        :disabled="!table.getCanNextPage()"
        @click="table.setPageIndex(table.getPageCount() - 1)"
      >
        <ChevronsRight class="h-4 w-4" aria-hidden="true" />
      </Button>
    </div>
  </div>
</template>
