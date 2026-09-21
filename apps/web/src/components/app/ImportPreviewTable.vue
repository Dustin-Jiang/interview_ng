<!--
  ImportPreviewTable —— 导入实时预览：统计条 + 三段对照（原始行 → 映射后 JSON → 校验状态）。
  默认只渲染前 10 行，可切换为全部；每行的原始值与映射结果只在映射结果变化时序列化一次。
-->
<script setup lang="ts">
import { computed, ref } from 'vue'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { ImportOutcome, ImportSheet } from '@/domain/import'

const props = defineProps<{
  sheet: ImportSheet
  outcome: ImportOutcome
}>()

/** 预览默认只展示前 10 行（2000 行全量渲染只在用户显式切换时发生）。 */
const PREVIEW_ROWS = 10
const showAll = ref(false)

interface PreviewRow {
  line: number
  raw: string
  mapped: string
  status: 'create' | 'update' | 'error'
  label: string
  errors: string[]
}

const rows = computed<PreviewRow[]>(() =>
  props.outcome.mapped.map((row, index) => ({
    line: row.line,
    raw: JSON.stringify(props.sheet.rows[index]?.values ?? {}),
    mapped: JSON.stringify({ student_no: row.studentNo, name: row.name, profile: row.profile }),
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

const visible = computed(() => (showAll.value ? rows.value : rows.value.slice(0, PREVIEW_ROWS)))
</script>

<template>
  <div class="space-y-3">
    <div class="flex flex-wrap items-center gap-2">
      <Badge variant="outline">总 {{ props.outcome.stats.total }}</Badge>
      <Badge variant="secondary">新建 {{ props.outcome.stats.create }}</Badge>
      <Badge variant="outline">更新 {{ props.outcome.stats.update }}</Badge>
      <Badge :variant="props.outcome.stats.failed ? 'destructive' : 'outline'">
        失败 {{ props.outcome.stats.failed }}
      </Badge>
      <Button
        v-if="rows.length > PREVIEW_ROWS"
        variant="link"
        class="ml-auto h-auto p-0"
        @click="showAll = !showAll"
      >
        {{ showAll ? `只看前 ${PREVIEW_ROWS} 行` : `显示全部 ${rows.length} 行` }}
      </Button>
    </div>

    <div class="overflow-x-auto rounded-md border">
      <Table class="min-w-[52rem]">
        <TableHeader>
          <TableRow>
            <TableHead class="w-16">行号</TableHead>
            <TableHead>原始行</TableHead>
            <TableHead>映射后</TableHead>
            <TableHead class="w-44">校验状态</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in visible" :key="row.line">
            <TableCell class="font-mono text-xs text-muted-foreground">{{ row.line }}</TableCell>
            <TableCell class="break-all font-mono text-xs">{{ row.raw }}</TableCell>
            <TableCell class="break-all font-mono text-xs">{{ row.mapped }}</TableCell>
            <TableCell>
              <div class="space-y-1">
                <Badge :variant="row.status === 'error' ? 'destructive' : 'secondary'">
                  {{ row.label }}
                </Badge>
                <p v-for="message in row.errors" :key="message" class="text-xs text-destructive">
                  {{ message }}
                </p>
              </div>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>
