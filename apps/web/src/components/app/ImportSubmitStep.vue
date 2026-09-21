<!--
  ImportSubmitStep —— 数据导入第 ③ 步：确认与提交。
  提交通知整批全或无：服务端返回即整批已落库（报告逐行给出新建/更新）；
  任一行不合法则整批拒绝，此时就地列出全部问题行（行号 + 原因）。
-->
<script setup lang="ts">
import { computed } from 'vue'
import { CheckCircle2, XCircle } from 'lucide-vue-next'

import ErrorAlert from '@/components/app/ErrorAlert.vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
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

/** 服务端行级错误：下标 → 源文件行号（提交载荷与映射结果同序）。 */
const problemRows = computed(() =>
  props.rowErrors.map((e) => ({
    line: props.mapped[e.index]?.line ?? e.index + 1,
    studentNo: props.mapped[e.index]?.studentNo ?? '',
    reason: e.error,
  })),
)

/** 落库报告：逐行结果（下标 → 源文件行号与学号）。 */
const resultRows = computed(() =>
  (props.report?.rows ?? []).map((row) => ({
    line: props.mapped[row.index]?.line ?? row.index + 1,
    studentNo: props.mapped[row.index]?.studentNo ?? '',
    status: row.status,
    candidateId: row.candidate_id,
  })),
)
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

    <Card v-if="props.report">
      <CardHeader>
        <CardTitle class="flex items-center gap-2">
          <CheckCircle2 class="h-4 w-4 text-muted-foreground" aria-hidden="true" />
          导入结果
        </CardTitle>
      </CardHeader>
      <CardContent class="space-y-3">
        <div class="flex flex-wrap items-center gap-2">
          <Badge variant="secondary">新建 {{ props.report.created }}</Badge>
          <Badge variant="outline">更新 {{ props.report.updated }}</Badge>
          <Button variant="link" class="ml-auto h-auto p-0" @click="emit('reset')">继续导入下一批</Button>
        </div>
        <div class="max-h-96 overflow-y-auto rounded-md border">
          <table class="w-full text-sm">
            <tbody>
              <tr v-for="row in resultRows" :key="row.line" class="border-b last:border-0">
                <td class="w-16 p-2 font-mono text-xs text-muted-foreground">{{ row.line }}</td>
                <td class="p-2 font-mono text-xs">{{ row.studentNo }}</td>
                <td class="w-20 p-2">
                  <Badge :variant="row.status === 'created' ? 'secondary' : 'outline'">
                    {{ row.status === 'created' ? '新建' : '更新' }}
                  </Badge>
                </td>
                <td class="w-24 p-2 text-right">
                  <RouterLink
                    class="text-primary underline-offset-4 hover:underline"
                    :to="{ name: 'candidate', params: { candidateId: String(row.candidateId) } }"
                  >
                    #{{ row.candidateId }}
                  </RouterLink>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </CardContent>
    </Card>

    <Card v-if="problemRows.length > 0">
      <CardHeader>
        <CardTitle class="flex items-center gap-2">
          <XCircle class="h-4 w-4 text-destructive" aria-hidden="true" />
          整批被拒绝
        </CardTitle>
      </CardHeader>
      <CardContent class="space-y-3">
        <ErrorAlert :message="`服务端返回 ${problemRows.length} 处问题，本批未落库任何数据。`" />
        <div class="max-h-96 overflow-y-auto rounded-md border">
          <table class="w-full text-sm">
            <tbody>
              <tr v-for="row in problemRows" :key="row.line" class="border-b last:border-0">
                <td class="w-16 p-2 font-mono text-xs text-muted-foreground">{{ row.line }}</td>
                <td class="p-2 font-mono text-xs">{{ row.studentNo || '-' }}</td>
                <td class="p-2 text-destructive">{{ row.reason }}</td>
              </tr>
            </tbody>
          </table>
        </div>
        <Button variant="outline" size="sm" @click="emit('back')">返回修改映射</Button>
      </CardContent>
    </Card>
  </div>
</template>
