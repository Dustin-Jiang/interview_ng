<!--
  WaitingBoardView —— 候场大屏（DataTable 版）：未完成名单 + 签到操作。
  与候选人管理界面共用 DataTable 视觉；差异：名单按状态排序
  （正在面试 > 等待开始 > 等待分配 > 其他，COMPLETED 不上屏），仅展示学号、姓名与房间，
  操作列只保留签到。签到仅对持有 candidates.checkin 权限的用户可见，名单对任意登录用户可见。
-->
<script setup lang="ts">
import { computed, h, onMounted } from 'vue'
import { toast } from 'vue-sonner'
import { UsersRound } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { useCandidates } from '@/composables/useCandidates'
import { useAuth } from '@/composables/useAuth'
import { useBoardRefresh } from '@/composables/useBoardChannel'
import { sortWaitingBoard } from '@/domain/status'
import { PERMISSIONS, type Candidate } from '@/models'
import { toastError } from '@/lib/toast'

import { Button } from '@/components/ui/button'
import type { DataTableFeatures } from '@/components/ui/table/features'
import DataTableSection from '@/components/app/DataTableSection.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const { hasPermission } = useAuth()
// 组合式函数（函数式 ViewModel）：顶层解构，模板直接引用（ref 自动解包）。
const { candidates, keyword, loading, load, checkin, setKeyword } = useCandidates()

const canCheckin = computed(() => hasPermission(PERMISSIONS.CANDIDATES_CHECKIN))

/** 候场名单：COMPLETED 不上屏，按状态优先级升序，组内按创建时间升序。 */
const boardCandidates = computed<readonly Candidate[]>(() => sortWaitingBoard(candidates.value))

const emptyText = computed(() => (keyword.value ? '没有匹配的候选人' : '暂无候选人'))

async function handleCheckin(c: Candidate) {
  try {
    await checkin(c.id)
    toast.success(`「${c.name}」已签到`)
  } catch (e) {
    toastError(e)
  }
}

// ---- DataTable 列定义（h() 渲染，闭包捕获视图处理函数） ----
const columnHelper = createColumnHelper<DataTableFeatures, Candidate>()
const columns: ColumnDef<DataTableFeatures, Candidate>[] = columnHelper.columns([
  columnHelper.accessor('student_no', {
    header: '学号',
    enableSorting: false,
    cell: ({ getValue }) => h('span', { class: 'font-mono' }, String(getValue())),
  }),
  columnHelper.accessor('name', {
    header: '姓名',
    enableSorting: false,
  }),
  columnHelper.accessor('room_id', {
    header: '房间',
    enableSorting: false,
    cell: ({ row }) => {
      const roomId = row.original.room_id
      if (!roomId) return h('span', { class: 'text-muted-foreground' }, '-')
      return h('span', { class: 'font-mono' }, `#${roomId}`)
    },
  }),
  columnHelper.display({
    id: 'actions',
    header: '操作',
    enableHiding: false,
    cell: ({ row }) => {
      const c = row.original
      if (c.status !== 'NOT_CHECKED_IN' || !canCheckin.value) {
        return h('span', { class: 'text-muted-foreground' }, '-')
      }
      return h(Button, { size: 'sm', variant: 'outline', onClick: () => handleCheckin(c) }, () => '签到')
    },
  }),
])

// ---- 实时刷新：看板通道事件（签到/拉取/阶段变化/CRUD）→ 防抖重拉名单 ----
useBoardRefresh(
  [
    'candidate_signed_in',
    'candidate_assigned',
    'candidate_created',
    'candidate_updated',
    'candidate_deleted',
    'room_phase_changed',
  ],
  () => void load(),
)

onMounted(() => void load())
</script>

<template>
  <PageShell title="候场大屏">
    <template #actions>
      <RefreshButton label="刷新候选人列表" :loading="loading" @click="load" />
    </template>

    <DataTableSection
      title="候场名单"
      :loading="loading"
      :items="candidates"
      :columns="columns"
      :data="boardCandidates"
      :empty-text="emptyText"
      :empty-icon="UsersRound"
    >
      <!-- 搜索框：图标 + 可清空（Enter / 清空均触发检索）。 -->
      <template #toolbar>
        <SearchInput v-model="keyword" placeholder="搜索学号 / 姓名…" @search="setKeyword" />
      </template>
    </DataTableSection>
  </PageShell>
</template>
