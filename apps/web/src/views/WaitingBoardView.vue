<!--
  WaitingBoardView —— 候场大屏（DataTable 版）：未完成名单 + 签到操作。
  与候选人管理界面共用 DataTable 视觉；差异：名单按状态分档
  （正在面试 > 等待开始 > 等待分配 > 其他，COMPLETED 不上屏），**档内按签到先后排序**（叫号次序），
  展示学号、姓名、状态与房间名，
  操作列只保留签到。签到仅对持有 candidates.checkin 权限的用户可见，名单对任意登录用户可见。
  候选人被拉取进房间（candidate_assigned）时弹叫号弹窗，大字报出姓名与目标房间名。
-->
<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { UsersRound } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { candidateApi } from '@/api/http'
import { useCandidates } from '@/composables/useCandidates'
import { useAuth } from '@/composables/useAuth'
import { useBoardChannel, useBoardRefresh } from '@/composables/useBoardChannel'
import { useRoomNames } from '@/composables/useRoomNames'
import { sortWaitingBoard } from '@/domain/status'
import { UNNAMED_ROOM_LABEL } from '@/domain/room'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { PERMISSIONS, type Candidate } from '@/models'
import { toastError } from '@/lib/toast'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { DataTableFeatures } from '@/components/ui/table/features'
import CallNumberDialog from '@/components/app/CallNumberDialog.vue'
import DataTableSection from '@/components/app/DataTableSection.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const { hasPermission } = useAuth()
// 组合式函数（函数式 ViewModel）：顶层解构，模板直接引用（ref 自动解包）。
const { candidates, keyword, loading, load, checkin, setKeyword } = useCandidates()

const canCheckin = computed(() => hasPermission(PERMISSIONS.CANDIDATES_CHECKIN))

/** 候场名单：COMPLETED 不上屏，按状态优先级分档，档内按签到先后（叫号次序）。 */
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

// ---- 叫号队列：候选人被拉取进房间时入队，弹窗逐条报号 ----
interface CallNotice {
  /** 自增键：同一候选人两次叫号也视为不同条目。 */
  key: number
  name: string
  studentNo: string
  roomLabel: string
}

const calls = ref<CallNotice[]>([])
const currentCall = computed(() => calls.value[0] ?? null)
/** 房间 id → 展示名（未命名显示「未命名」，不显示编号）。 */
const { roomLabelOf } = useRoomNames()
let callSeq = 0

/**
 * 入队一次叫号。候选人可能不在当前榜单里（关键词过滤后服务端只回匹配项），
 * 此时按 id 兜底取一次；候选人已被删除则无号可叫。
 */
async function enqueueCall(candidateId: number, roomId: number): Promise<void> {
  const local = candidates.value.find((c) => c.id === candidateId)
  const c = local ?? (await candidateApi.get(candidateId).catch(() => null))
  if (!c) return
  callSeq += 1
  calls.value = [
    ...calls.value,
    {
      key: callSeq,
      name: c.name,
      studentNo: c.student_no,
      roomLabel: roomLabelOf(roomId),
    },
  ]
}

function dismissCall(): void {
  calls.value = calls.value.slice(1)
}

// ---- DataTable 列定义（h() 渲染，闭包捕获视图处理函数） ----
const columnHelper = createColumnHelper<DataTableFeatures, Candidate>()
const columns: ColumnDef<DataTableFeatures, Candidate>[] = columnHelper.columns([
  columnHelper.accessor('student_no', {
    header: '学号',
    enableSorting: false,
    cell: ({ getValue }) => h('span', { class: 'tabular-nums' }, String(getValue())),
  }),
  columnHelper.accessor('name', {
    header: '姓名',
    enableSorting: false,
  }),
  columnHelper.accessor('status', {
    header: '状态',
    enableSorting: false,
    cell: ({ getValue }) => {
      const presentation = STATUS_PRESENTATION[getValue()]
      return h(Badge, { variant: presentation.badge, class: 'text-sm' }, () => presentation.label)
    },
  }),
  columnHelper.accessor('room_id', {
    header: '房间',
    enableSorting: false,
    cell: ({ row }) => {
      const roomId = row.original.room_id
      if (!roomId) return h('span', { class: 'text-muted-foreground' }, '-')
      // 大屏只报房间名（不显示编号）。不写死 nowrap：<md 卡片模式靠 whitespace-normal 生效，
      // 写死 nowrap 会把卡片顶宽、导致横向溢出。
      const label = roomLabelOf(roomId)
      const unnamed = label === UNNAMED_ROOM_LABEL
      return h('span', { class: unnamed ? 'text-muted-foreground' : '' }, label)
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

// 叫号：拉取事件载荷为 {CandidateID, RoomID}，直接入队（房间名由 useRoomNames 自持刷新）。
const { subscribe } = useBoardChannel()
subscribe((ev) => {
  if (ev.type !== 'candidate_assigned') return
  const data = ev.data as { CandidateID?: number; RoomID?: number } | undefined
  if (data?.CandidateID && data.RoomID) void enqueueCall(data.CandidateID, data.RoomID)
})

onMounted(() => {
  void load()
})
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

    <!-- 叫号弹窗：拉取进房间时弹出，倒计时自动关闭 -->
    <CallNumberDialog
      :notice="currentCall"
      :pending="Math.max(0, calls.length - 1)"
      @dismiss="dismissCall"
    />
  </PageShell>
</template>
