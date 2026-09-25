<!--
  WaitingBoardView —— 候场大屏（DataTable 版）：未完成名单 + 签到操作。
  与候选人管理界面共用 DataTable 视觉；差异：名单按状态分档
  （正在面试 > 等待开始 > 等待分配 > 其他，COMPLETED 不上屏），
  **档内按候场队列口径排序**（见 domain/status.ts#compareWaiting：手动优先级 → 签到先后 → 添加顺序），
  展示序号（队列名次）、学号、姓名、状态与房间名。
  操作列：签到（未签到者）+ 上移/下移（**仅「已签到待分配」档**——它是候场队列的唯一消费方；
  正在面试的顺序由面试进程决定，未签到者还没到队，调了没有读取方）。
  签到与调序都只对持有 candidates.checkin 权限的用户可见，名单对任意登录用户可见。
  候选人被拉取进房间（candidate_assigned）时弹叫号弹窗，大字报出姓名与目标房间名。
-->
<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { ArrowDown, ArrowUp, UsersRound } from 'lucide-vue-next'
import { createColumnHelper, type ColumnDef } from '@tanstack/vue-table'

import { candidateApi } from '@/api/http'
import { useCandidates } from '@/composables/useCandidates'
import { useAuth } from '@/composables/useAuth'
import { useBoardChannel, useBoardRefresh } from '@/composables/useBoardChannel'
import { useRooms } from '@/composables/useRooms'
import { ROSTER_BOARD_EVENTS, sortWaitingBoard } from '@/domain/status'
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
// board: true —— 只拉未定局的档位（大屏不上屏的面试已结束/录取档不参与这次拉取）。
const { candidates, keyword, loading, load, checkin, setKeyword } = useCandidates({ board: true })

const canCheckin = computed(() => hasPermission(PERMISSIONS.CANDIDATES_CHECKIN))

/** 候场名单：不上屏的档过滤掉，按状态优先级分档，档内按候场队列口径（手动优先级 → 签到先后）。 */
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

/** 手动调序权限：与签到同档（candidates.checkin，大屏操作者）。 */
const canAdjust = computed(() => hasPermission(PERMISSIONS.CANDIDATES_CHECKIN))

/**
 * 上/下一位：不动本地顺序，以服务端为权威。写成功后**本地立即重拉**（与签到同一手法），
 * 其余客户端由 `candidate_priority_changed` 事件防抖重拉——不能只等事件：看板通道
 * 需要 `rooms.view`，没有该权限的操作者（只有签到/调序权限）拿不到事件，界面会停在旧顺序。
 * moved = false（已在档首/档尾）同样重拉：那是本地快照过期，服务端顺序才是权威，且不报错。
 */
async function handleMove(c: Candidate, direction: 'up' | 'down') {
  try {
    await candidateApi.setWaitingPriority(c.id, direction)
    await load()
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
/** 房间 id → 展示名（未命名显示「未命名」，不显示编号）。名单取自 useRooms（共享数据源）。 */
const { roomLabelOf, load: loadRooms } = useRooms()
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
/**
 * 序号列 = 榜单显示序（含手动优先级效果；纯列序，不是候选人的字段）。
 * **未签到者不出序号**：他们还没进候场队列（不参与叫号，也没有签到时刻），
 * 显示名次会让人误以为已排上队；而未签到的档位排在榜单最后，故在流程中的人
 * 序号仍是连续的 1..N。
 * 刻意留空列头：<md 卡片模式下「首个有列头的列」（学号）才是卡片标题，
 * 本列因此并入标题行右侧显示为名次徽章，不会顶掉标识列。
 */
const seqColumn: ColumnDef<DataTableFeatures, Candidate> = columnHelper.display({
  id: 'seq',
  header: '',
  enableHiding: false,
  cell: ({ row }) => {
    if (row.original.status === 'NOT_CHECKED_IN') {
      return h('span', { class: 'text-muted-foreground' }, '-')
    }
    return h(
      'span',
      {
        class:
          'inline-flex size-6 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold tabular-nums text-primary',
      },
      String(row.index + 1),
    )
  },
})

const columns = computed<ColumnDef<DataTableFeatures, Candidate>[]>(() =>
  columnHelper.columns([
    columnHelper.accessor('student_no', {
      header: '学号',
      enableSorting: false,
      cell: ({ getValue }) => h('span', { class: 'tabular-nums' }, String(getValue())),
    }),
    // 有关键词过滤时榜单只是匹配子集，「第 N 位」不再等于队列名次 → 隐藏序号列（不误导）。
    ...(keyword.value ? [] : [seqColumn]),
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
        const cells: ReturnType<typeof h>[] = []
        // 调序只对候场队列（已签到待分配）开放：其余档位没有读取方（见后端 AdjustWaitingPriority）。
        // 不做本地「相邻判断/禁用」：开着关键词过滤时本地相邻 ≠ 服务端相邻，且多人同时操作时
        // 快照必然过期——由服务端幂等裁决（moved=false 静默重拉），这里只负责给入口。
        if (c.status === 'CHECKED_IN_PENDING_ASSIGN' && canAdjust.value) {
          cells.push(
            h(
              Button,
              {
                size: 'sm',
                variant: 'ghost',
                'aria-label': `把「${c.name}」提前一位`,
                onClick: () => void handleMove(c, 'up'),
              },
              () => h(ArrowUp, { class: 'h-4 w-4' }),
            ),
            h(
              Button,
              {
                size: 'sm',
                variant: 'ghost',
                'aria-label': `把「${c.name}」推后一位`,
                onClick: () => void handleMove(c, 'down'),
              },
              () => h(ArrowDown, { class: 'h-4 w-4' }),
            ),
          )
        }
        if (c.status === 'NOT_CHECKED_IN' && canCheckin.value) {
          cells.push(h(Button, { size: 'sm', variant: 'outline', onClick: () => handleCheckin(c) }, () => '签到'))
        }
        if (cells.length === 0) return h('span', { class: 'text-muted-foreground' }, '-')
        return h('div', { class: 'flex items-center justify-end gap-1' }, cells)
      },
    }),
  ]),
)

// ---- 实时刷新：看板通道事件（签到/拉取/阶段变化/CRUD）→ 防抖重拉名单 ----
useBoardRefresh(ROSTER_BOARD_EVENTS, () => void load())

// 叫号：拉取事件载荷为 {CandidateID, RoomID}，直接入队（房间名由 useRooms 自持刷新）。
const { subscribe } = useBoardChannel()
subscribe((ev) => {
  if (ev.type !== 'candidate_assigned') return
  const data = ev.data as { CandidateID?: number; RoomID?: number } | undefined
  if (data?.CandidateID && data.RoomID) void enqueueCall(data.CandidateID, data.RoomID)
})

onMounted(() => {
  void load()
  // 房间名映射与候场名单分开拉：两条数据源各自刷新（房间 CRUD 事件由 useRooms 自己订阅）。
  void loadRooms()
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
