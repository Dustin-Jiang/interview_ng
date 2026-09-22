<script setup lang="ts">
import type { Column } from '@tanstack/vue-table'
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-vue-next'
import type { DataTableFeatures } from './features'
import { Button } from '@/components/ui/button'

const props = defineProps<{
  column: Column<DataTableFeatures, any>
  title: string
}>()
</script>

<template>
  <Button
    v-if="column.getCanSort()"
    variant="ghost"
    size="sm"
    class="-ml-3 h-8 text-sm"
    @click="column.toggleSorting(column.getIsSorted() === 'asc')"
  >
    <span>{{ title }}</span>
    <ArrowDown v-if="column.getIsSorted() === 'desc'" class="ml-2 h-4 w-4" />
    <ArrowUp v-else-if="column.getIsSorted() === 'asc'" class="ml-2 h-4 w-4" />
    <ArrowUpDown v-else class="ml-2 h-4 w-4" />
  </Button>
  <div v-else>{{ title }}</div>
</template>
