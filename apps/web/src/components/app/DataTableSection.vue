<!--
  DataTableSection —— 管理列表区块：骨架 / 空态 / 数据表三段式（+ 可选标题与工具条）。
  收编各设置分区与候场大屏重复的「首屏骨架 → 空态 → DataTable」结构；
  工具条（搜索、状态下拉等）由 #toolbar 插槽提供。
-->
<script setup lang="ts">
import type { Component } from 'vue'
import type { ColumnDef } from '@tanstack/vue-table'

import DataTable from '@/components/ui/table/data-table.vue'
import type { DataTableFeatures } from '@/components/ui/table/features'
import EmptyState from '@/components/app/EmptyState.vue'
import ListSkeleton from '@/components/app/ListSkeleton.vue'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    /** 拉取中（列表为空时展示骨架，刷新时不打断已有内容）。 */
    loading: boolean
    /** 判定空态的数据源（通常为未筛选的列表）。 */
    items: readonly unknown[]
    columns: ColumnDef<DataTableFeatures, any>[]
    data: readonly any[]
    emptyText: string
    emptyIcon?: Component
    /** 区块标题（省略则不输出标题行）。 */
    title?: string
    /** 表头不换行（各列内容 nowrap + 容器横向滚动）。 */
    nowrapHeaders?: boolean
    skeletonRows?: number
    skeletonItemClass?: string
  }>(),
  {
    emptyIcon: undefined,
    title: '',
    nowrapHeaders: false,
    skeletonRows: 5,
    skeletonItemClass: 'h-12 w-full rounded-md',
  },
)
</script>

<template>
  <div :class="cn('space-y-3', props.nowrapHeaders && '[&_th]:whitespace-nowrap')">
    <div v-if="props.title || $slots.toolbar" class="flex flex-wrap items-center justify-between gap-3">
      <h2 v-if="props.title" class="text-sm font-semibold">{{ props.title }}</h2>
      <slot name="toolbar" />
    </div>

    <ListSkeleton
      v-if="props.loading && props.items.length === 0"
      :rows="props.skeletonRows"
      :item-class="props.skeletonItemClass"
    />

    <EmptyState v-else-if="props.items.length === 0" :icon="props.emptyIcon">
      {{ props.emptyText }}
    </EmptyState>

    <DataTable v-else :columns="props.columns" :data="props.data" />
  </div>
</template>
