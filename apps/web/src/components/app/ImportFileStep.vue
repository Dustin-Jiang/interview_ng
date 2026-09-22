<!--
  ImportFileStep —— 数据导入第 ① 步：选择工作簿。
  解析与边界校验（.xlsx / 体积 / 行数 / 表头）在 domain/import.ts，此处只呈现结果与原因。
-->
<script setup lang="ts">
import { FileSpreadsheet } from 'lucide-vue-next'

import ErrorAlert from '@/components/app/ErrorAlert.vue'
import FileDropInput from '@/components/app/FileDropInput.vue'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import type { ImportSheet } from '@/domain/import'

const props = defineProps<{
  sheet: ImportSheet | null
  parsing: boolean
  error: string | null
}>()

const emit = defineEmits<{ pick: [file: File]; clear: []; next: [] }>()
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>选择工作簿</CardTitle>
    </CardHeader>
    <CardContent class="space-y-3">
      <FileDropInput :disabled="props.parsing" @pick="emit('pick', $event)">
        <span v-if="props.parsing" class="flex items-center gap-2 text-muted-foreground">
          <Spinner aria-hidden="true" />
          正在解析…
        </span>
        <code v-else-if="props.sheet" class="break-all font-mono text-xs">{{ props.sheet.fileName }}</code>
      </FileDropInput>

      <ErrorAlert v-if="props.error" :message="props.error" />

      <!-- 工作簿摘要 + 操作：窄屏下摘要行允许换行，长表名（无空格）在行内断行，不能把按钮挤出容器。 -->
      <div v-if="props.sheet" class="flex flex-wrap items-center justify-between gap-3">
        <p class="flex min-w-0 flex-wrap items-center gap-2 text-sm">
          <FileSpreadsheet class="h-4 w-4 text-muted-foreground" aria-hidden="true" />
          <span class="min-w-0 break-all font-medium">{{ props.sheet.sheetName }}</span>
          <span class="font-mono text-xs text-muted-foreground">
            {{ props.sheet.headers.length }} 列 × {{ props.sheet.rows.length }} 行
          </span>
        </p>
        <div class="flex shrink-0 items-center gap-2">
          <Button variant="outline" size="sm" @click="emit('clear')">重新选择</Button>
          <Button size="sm" @click="emit('next')">下一步</Button>
        </div>
      </div>
    </CardContent>
  </Card>
</template>
