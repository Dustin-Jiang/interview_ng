<!--
  ImportSubmitStep —— 数据导入第 ③ 步：确认与提交。
  提交通知整批全或无：服务端返回即整批已落库（报告逐行给出新建/更新）；
  任一行不合法则整批拒绝，此时就地列出全部问题行（行号 + 原因）。
  报告表沿用列表页的表格区块（分页、不嵌套 Card），大批次不会一次渲染上千行。
-->
<script setup lang="ts">
import { computed, h } from 'vue'
import { RouterLink } from 'vue-router'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import DataTableSection from '@/components/app/DataTableSection.vue'
import ErrorAlert from '@/components/app/ErrorAlert.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import type { DataTableFeatures } from '@/components/ui/table/features'
import type { ImportMappedRow } from '@/domain/import'
import type { CandidateImportReport, CandidateImportRowError } from '@/models'

const props = defineProps<{
  mapped: readonly ImportMappedRow[]
  submitting: boolean
  report: CandidateImportReport | null
  rowErrors: readonly CandidateImportRowError[]
}>()

const emit = defineEmits<{ submit: []; reset: []; back: [] }>()

/** 统计（与预览、服务端报告同一口径）。 */
const stats = computed(() => ({
  total: props.mapped.length,
  create: props.mapped.filter((r) => r.status === 'create').length,
  update: props.mapped.filter((r) => r.status === 'update').length,
}))

interface ResultRow {
  line: number
  studentNo: string
  created: boolean
  candidateId: number
}

/** 落库报告：逐行结果（下标 → 源文件行号与学号）。 */
const resultRows = computed<ResultRow[]>(() =>
  (props.report?.rows ?? []).map((row) => ({
    line: props.mapped[row.index]?.line ?? row.index + 1,
    studentNo: props.mapped[row.index]?.studentNo ?? '',
    created: row.status === 'created',
    candidateId: row.candidate_id,
  })),
)

interface ProblemRow {
  line: number
  studentNo: string
  reason: string
}

/** 服务端行级错误：下标 → 源文件行号（提交载荷与映射结果同序）。 */
const problemRows = computed<ProblemRow[]>(() =>
  props.rowErrors.map((e) => ({
    line: props.mapped[e.index]?.line ?? e.index + 1,
    studentNo: props.mapped[e.index]?.studentNo ?? '',
    reason: e.error,
  })),
)

const columnHelper = createColumnHelper<DataTableFeatures, ResultRow>()
const resultColumns: ColumnDef<DataTableFeatures, ResultRow>[] = columnHelper.columns([
  columnHelper.accessor('line', {
    header: '行号',
    enableSorting: false,
    cell: ({ getValue }) => h('div', { class: 'font-mono text-xs text-muted-foreground' }, String(getValue())),
  }),
  columnHelper.accessor('studentNo', {
    header: '学号',
    enableSorting: false,
    cell: ({ getValue }) => h('div', { class: 'font-mono text-xs' }, String(getValue())),
  }),
  columnHelper.accessor('created', {
    header: '结果',
    enableSorting: false,
    cell: ({ getValue }) =>
      h(Badge, { variant: getValue() ? 'secondary' : 'outline' }, () => (getValue() ? '新建' : '更新')),
  }),
  columnHelper.display({
    id: 'candidate',
    header: '候选人',
    enableHiding: false,
    cell: ({ row }) =>
      h(RouterLink, { class: 'text-primary underline-offset-4 hover:underline', to: { name: 'candidate', params: { candidateId: String(row.original.candidateId) } } }, () => `#${row.original.candidateId}`),
  }),
])

const problemColumnHelper = createColumnHelper<DataTableFeatures, ProblemRow>()
const problemColumns: ColumnDef<DataTableFeatures, ProblemRow>[] = problemColumnHelper.columns([
  problemColumnHelper.accessor('line', {
    header: '行号',
    enableSorting: false,
    cell: ({ getValue }) => h('div', { class: 'font-mono text-xs text-muted-foreground' }, String(getValue())),
  }),
  problemColumnHelper.accessor('studentNo', {
    header: '学号',
    enableSorting: false,
    cell: ({ getValue }) => {
      const value = String(getValue() ?? '')
      return value ? h('div', { class: 'font-mono text-xs' }, value) : h('span', { class: 'text-muted-foreground' }, '-')
    },
  }),
  problemColumnHelper.accessor('reason', {
    header: '原因',
    enableSorting: false,
    cell: ({ getValue }) => h('div', { class: 'text-destructive' }, String(getValue())),
  }),
])
</script>

<template>
  <div class="space-y-4">
    <Card v-if="!props.report && problemRows.length === 0">
      <CardHeader>
        <CardTitle>确认导入</CardTitle>
      </CardHeader>
      <CardContent class="space-y-3">
        <div class="flex flex-wrap items-center gap-2">
          <Badge variant="outline">总 {{ stats.total }}</Badge>
          <Badge variant="secondary">新建 {{ stats.create }}</Badge>
          <Badge variant="outline">更新 {{ stats.update }}</Badge>
        </div>
        <div class="flex items-center gap-2">
          <Button variant="outline" size="sm" @click="emit('back')">返回修改</Button>
          <Button size="sm" :disabled="props.submitting" @click="emit('submit')">
            <Spinner v-if="props.submitting" aria-hidden="true" />
            导入 {{ stats.total }} 行
          </Button>
        </div>
      </CardContent>
    </Card>

    <DataTableSection
      v-if="props.report"
      title="导入结果"
      :loading="false"
      :items="resultRows"
      :columns="resultColumns"
      :data="resultRows"
      empty-text="本次导入没有落库任何行"
    >
      <template #toolbar>
        <div class="flex flex-wrap items-center gap-2">
          <Badge variant="secondary">新建 {{ props.report.created }}</Badge>
          <Badge variant="outline">更新 {{ props.report.updated }}</Badge>
          <Button variant="link" class="h-auto p-0" @click="emit('reset')">继续导入下一批</Button>
        </div>
      </template>
    </DataTableSection>

    <template v-if="problemRows.length > 0">
      <ErrorAlert :message="`服务端返回 ${problemRows.length} 处问题，本批未落库任何数据。`" />
      <DataTableSection
        title="整批被拒绝"
        :loading="false"
        :items="problemRows"
        :columns="problemColumns"
        :data="problemRows"
        empty-text="没有问题行"
      >
        <template #toolbar>
          <Button variant="outline" size="sm" @click="emit('back')">返回修改映射</Button>
        </template>
      </DataTableSection>
    </template>
  </div>
</template>
