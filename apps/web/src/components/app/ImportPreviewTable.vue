<!--
  ImportPreviewTable —— 导入实时预览：统计条 + 逐行映射结果（学号 / 姓名 / 个人简介）+ 校验状态。
  映射结果按目标字段展开成列（不展示源表原始行 —— 源数据在 Excel 里，这里只核对将要落库的字段）。
  默认只渲染前 10 行，可切换为全部。
-->
<script setup lang="ts">
import { computed, ref } from 'vue'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { ImportOutcome } from '@/domain/import'

const props = defineProps<{
  outcome: ImportOutcome
}>()

/** 预览默认只展示前 10 行（2000 行全量渲染只在用户显式切换时发生）。 */
const PREVIEW_ROWS = 10
const showAll = ref(false)

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

    <!-- 不设最小宽度：设置页外壳（overflow-hidden）在窄屏下会裁切超宽内容，
         让表格按可用宽度收缩换行，比溢出被裁掉更可用。 -->
    <div class="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-16">行号</TableHead>
            <TableHead class="w-32">学号</TableHead>
            <TableHead class="w-28">姓名</TableHead>
            <TableHead>个人简介</TableHead>
            <TableHead class="w-44">校验状态</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="row in visible" :key="row.line">
            <TableCell class="font-mono text-xs text-muted-foreground">{{ row.line }}</TableCell>
            <TableCell class="font-mono text-xs">
              <span v-if="row.studentNo">{{ row.studentNo }}</span>
              <span v-else class="text-muted-foreground">-</span>
            </TableCell>
            <TableCell class="break-words">
              <span v-if="row.name">{{ row.name }}</span>
              <span v-else class="text-muted-foreground">-</span>
            </TableCell>
            <TableCell class="break-words text-muted-foreground">
              {{ row.profile || '-' }}
            </TableCell>
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
