<!--
  CandidateRecordsView —— 候选人查看页（与「候选人管理」分离，对普通用户开放）。
  左侧：候选人名册（搜索 + 状态筛选 + 键盘可达的 listbox）；右侧：选中候选人的详细资料与面试过程记录。
  移动端为「列表 ↔ 详情」两段式导航（选中候选人后进入详情，可返回列表）；桌面端左右分栏。
  筛选条件与选中条目分别同步到 URL query（`?q=`/`?status=`）与路径参数（`/candidates/:candidateId`，可深链 / 分享）；
  记录经 REST GET /api/candidates/:id/messages 拉取，并可在本页补充（POST 同一路径，需 rooms.chat）——
  归档写入不依赖房间与在场成员，面试结档后照样能补记录。

  组装层：布局与名册交给 MasterDetailSplit / RosterToolbar / RosterList / RosterPager，
  数据与交互交给 useCandidatePool / useRosterSelection / useRosterHotkeys / useCandidateMessages / useAdmissions。
-->
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ExternalLink, Pencil, SearchX, SlidersHorizontal, UsersRound, X } from 'lucide-vue-next'

import { useAdmissions } from '@/composables/useAdmissions'
import { useAuth } from '@/composables/useAuth'
import { useBoardChannel, useBoardRefresh } from '@/composables/useBoardChannel'
import { useCandidateMessages } from '@/composables/useCandidateMessages'
import { useCandidatePool } from '@/composables/useCandidatePool'
import { useRosterHotkeys } from '@/composables/useRosterHotkeys'
import { useRosterRouteSync } from '@/composables/useRosterRouteSync'
import { useRosterSelection } from '@/composables/useRosterSelection'
import { useRoomNames } from '@/composables/useRoomNames'
import { ADMISSION_PRESENTATION, STATUS_PRESENTATION } from '@/presenters/status'
import {
  ADMISSION_STATUSES,
  CANDIDATE_STATUSES,
  PERMISSIONS,
  type Candidate,
  type CandidateStatus,
} from '@/models'
import { formatDateTime } from '@/lib/format'
import { toastError } from '@/lib/toast'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { segmentedItemVariants } from '@/components/ui/tokens'
import CandidateDetailHeader from '@/components/app/CandidateDetailHeader.vue'
import CandidatePreferenceDialog from '@/components/app/CandidatePreferenceDialog.vue'
import CandidateStatusRadio from '@/components/app/CandidateStatusRadio.vue'
import EmptyState from '@/components/app/EmptyState.vue'
import ErrorAlert from '@/components/app/ErrorAlert.vue'
import ListSkeleton from '@/components/app/ListSkeleton.vue'
import MasterDetailSplit from '@/components/app/MasterDetailSplit.vue'
import MessageComposer from '@/components/app/MessageComposer.vue'
import MessageTranscript from '@/components/app/MessageTranscript.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'
import RosterList from '@/components/app/RosterList.vue'
import RosterPager from '@/components/app/RosterPager.vue'
import RosterToolbar from '@/components/app/RosterToolbar.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const route = useRoute()
const router = useRouter()
const { user, hasPermission } = useAuth()

// ---- 名册数据（一次拉取，客户端筛选） ----
const { candidates, loading: poolLoading, error: poolError, load } = useCandidatePool()

/** 搜索关键词（学号 / 姓名 / 简介包含匹配）。 */
const keyword = ref('')
/** 状态筛选：'' = 全部。 */
const statusFilter = ref<'' | CandidateStatus>('')
/** 筛选浮层开关。 */
const filterOpen = ref(false)

/** 是否处于任一筛选激活态（用于统计条与空态文案）。 */
const hasFilter = computed(() => Boolean(keyword.value.trim() || statusFilter.value))

/** 客户端过滤后的名册（关键词 + 状态）。 */
const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return candidates.value.filter((c) => {
    if (statusFilter.value && c.status !== statusFilter.value) return false
    if (!kw) return true
    return (
      c.student_no.includes(kw) ||
      c.name.toLowerCase().includes(kw) ||
      (c.profile ?? '').toLowerCase().includes(kw)
    )
  })
})

function clearFilters() {
  keyword.value = ''
  statusFilter.value = ''
}

// ---- 选中 / 切换 / 键盘导航 / 移动端两段式 ----
const {
  selectedId,
  selected,
  select,
  canPrev,
  canNext,
  goPrev,
  goNext,
  showDetail,
  closeDetail,
  presetSelection,
} = useRosterSelection<Candidate>({
  items: () => filtered.value,
  lookup: (id) => candidates.value.find((c) => c.id === id) ?? null,
  ready: () => !poolLoading.value,
})

// ---- 深链：挂载恢复筛选（query）与选中条目（路径参数）；变更写回 URL（replace） ----
useRosterRouteSync<Candidate>({
  listRoute: 'candidates',
  itemRoute: 'candidate',
  selection: { selectedId, select, presetSelection },
  lookup: (id) => candidates.value.find((c) => c.id === id) ?? null,
  query: filterQuery,
  onMount: () => {
    const q = route.query.q
    const st = route.query.status
    if (typeof q === 'string') keyword.value = q
    if (typeof st === 'string' && (CANDIDATE_STATUSES as readonly string[]).includes(st)) {
      statusFilter.value = st as CandidateStatus
    }
    void load()
  },
})

/** 筛选态（query）——选中条目走路径参数。 */
function filterQuery(): Record<string, string> {
  const query: Record<string, string> = {}
  const kw = keyword.value.trim()
  if (kw) query.q = kw
  if (statusFilter.value) query.status = statusFilter.value
  return query
}

// ---- 录取决定（录取/捡漏阶段展示；按部门分别记录） ----
const {
  loading: admissionsLoading,
  canBrowseAll,
  canRecord,
  showControls,
  statusOf: admissionStatusOf,
  ownStatus,
  othersOf,
  switchAdmission,
} = useAdmissions(() => selected.value)

// ---- 键盘：↑/↓ 切换候选人（与捡漏页同一套键位）；1/2/3 记录本部门录取决定 ----
useRosterHotkeys({
  goPrev,
  goNext,
  onOtherKey: (e) => {
    const key = Number(e.key)
    if (!Number.isInteger(key) || key < 1 || key > ADMISSION_STATUSES.length) return false
    const candidate = selected.value
    if (!showControls.value || !canRecord.value || admissionsLoading.value) return false
    if (!candidate || !user.value?.department_id) return false
    void switchAdmission(candidate, ADMISSION_STATUSES[key - 1])
    return true
  },
})

// ---- 面试过程记录 ----
const { messages, loading: msgsLoading, error: msgsError, sending: msgsSending, load: loadMessages, prefetch: prefetchMessages, send: sendMessage } = useCandidateMessages()

/**
 * 补充记录（归档写入）：面试结档、房间解绑之后，记录仍可在本页补齐，故入口只按权限收口
 * （与房间聊天同一枚 `rooms.chat`），不看候选人档位。
 */
const canCompose = computed(() => hasPermission(PERMISSIONS.ROOMS_CHAT))
const draft = ref('')

async function submitMessage(): Promise<void> {
  const id = selectedId.value
  const text = draft.value.trim()
  if (!id || !text || msgsSending.value) return
  try {
    await sendMessage(id, text)
    draft.value = ''
  } catch (e) {
    toastError(e)
  }
}

watch(selectedId, (id) => {
  // 切人即清空草稿：补充的记录只属于当前选中的候选人，避免误写到下一位身上。
  draft.value = ''
  if (!id) return
  void loadMessages(id)
  // 预取相邻候选人记录：↑/↓ 连续浏览几乎全程命中缓存，切换零等待。
  const idx = filtered.value.findIndex((c) => c.id === id)
  if (idx !== -1) {
    prefetchMessages([filtered.value[idx - 1]?.id, filtered.value[idx + 1]?.id].filter((v): v is number => typeof v === 'number'))
  }
})

function reloadMessages() {
  if (selectedId.value) void loadMessages(selectedId.value)
}

// ---- 实时刷新（看板通道）：名册状态/CRUD → 防抖重拉；选中候选人有新消息 → 增量补拉 ----
const BOARD_ROSTER_EVENTS = [
  'candidate_signed_in',
  'candidate_assigned',
  'candidate_created',
  'candidate_updated',
  'candidate_deleted',
  'room_phase_changed',
]
useBoardRefresh(BOARD_ROSTER_EVENTS, () => void load())
useBoardChannel().subscribe((ev) => {
  // 归档的任何变动（新增 / 编辑 / 撤回 / 表情回复）都以重拉收口：服务端是唯一权威。
  if (
    ev.type !== 'message_appended' &&
    ev.type !== 'message_updated' &&
    ev.type !== 'message_deleted' &&
    ev.type !== 'message_reactions_changed'
  ) {
    return
  }
  const cid = (ev.data as { CandidateID?: number } | undefined)?.CandidateID
  if (cid != null && cid === selectedId.value) void loadMessages(cid)
})

// ---- 深链与写回见上方（onMounted 恢复 + useUrlSync） ----

// ---- 房间展示名：候选人身上只有 room_id，按 id 反查名字（未命名显示「未命名」） ----
const { roomLabelOf } = useRoomNames()

function goRoom(c: Candidate) {
  if (!c.room_id) return
  void router.push({ name: 'room', params: { roomId: String(c.room_id) } })
}

/**
 * 「面试房间」展示名：面试结束时记录的、这场面试所在房间。
 * 优先用当时留下的名字快照（房间之后改名/删除，也仍是「当时在哪间面的」）；
 * 快照为空（当时未命名）时按 id 反查房间列表。没有记录（还没面完）→ 空串，整行不渲染。
 */
const interviewRoomLabel = computed(() => {
  const c = selected.value
  if (!c) return ''
  return c.interview_room_name || roomLabelOf(c.interview_room_id)
})

// ---- 志愿与调剂：独立小权限（面试官默认持有），改完按 id 重拉名册即刷新详情 ----
const canEditPreferences = computed(() => hasPermission(PERMISSIONS.CANDIDATES_PREFERENCES))
const preferenceTarget = ref<Candidate | null>(null)

function openPreferences(): void {
  if (selected.value) preferenceTarget.value = selected.value
}

function onPreferencesSaved(): void {
  void load()
}
</script>

<template>
  <MasterDetailSplit
    :show-detail="showDetail"
    :detail-key="selectedId"
    aside-label="候选人名册"
    detail-label="候选人详情"
    back-aria-label="返回候选人列表"
    @back="closeDetail"
  >
    <!-- 左侧：名册工具条 + 名册 -->
    <template #aside>
      <!-- 紧凑工具条：标题 + 结果数 + 筛选浮层 + 刷新 -->
      <RosterToolbar title="候选人" :icon="UsersRound">
        <template #meta>
          <span class="shrink-0 text-xs text-muted-foreground" aria-live="polite">
            {{ filtered.length }} / {{ candidates.length }}
          </span>
        </template>
        <template #actions>
          <Popover :open="filterOpen" @update:open="filterOpen = $event">
            <PopoverTrigger as-child>
              <Button
                variant="ghost"
                size="icon"
                class="h-8 w-8"
                :class="hasFilter ? 'text-primary' : ''"
                aria-label="筛选候选人"
              >
                <SlidersHorizontal class="h-4 w-4" aria-hidden="true" />
              </Button>
            </PopoverTrigger>
            <!-- 手机上占满可用宽度（左右各留 1rem），≥sm 回到 18rem -->
            <PopoverContent class="w-[calc(100vw-2rem)] sm:w-72">
              <div class="space-y-3">
                <SearchInput v-model="keyword" full placeholder="搜索学号 / 姓名 / 简介…" />
                <CandidateStatusRadio v-model="statusFilter" allow-all />
                <Button
                  v-if="hasFilter"
                  variant="ghost"
                  size="sm"
                  class="w-full gap-1 text-muted-foreground"
                  @click="clearFilters"
                >
                  <X class="h-3.5 w-3.5" aria-hidden="true" />
                  清除筛选
                </Button>
              </div>
            </PopoverContent>
          </Popover>
          <RefreshButton variant="ghost" class="h-8 w-8" label="刷新列表" :loading="poolLoading" @click="load" />
        </template>
      </RosterToolbar>

      <RosterList
        :items="filtered"
        :selected-id="selectedId"
        :skeleton="poolLoading && candidates.length === 0"
        :error="poolError"
        :empty-text="hasFilter ? '没有匹配的候选人' : '暂无候选人'"
        :empty-icon="hasFilter ? SearchX : UsersRound"
        :empty-action-label="hasFilter ? '清除筛选' : ''"
        list-label="候选人列表"
        @select="select"
        @retry="load"
        @empty-action="clearFilters"
      >
        <template #badges="{ item }">
          <Badge
            v-if="showControls && admissionStatusOf(item.id)"
            :variant="ADMISSION_PRESENTATION[admissionStatusOf(item.id)!].badge"
          >
            本部门{{ ADMISSION_PRESENTATION[admissionStatusOf(item.id)!].label }}
          </Badge>
          <Badge :variant="STATUS_PRESENTATION[item.status].badge">
            {{ STATUS_PRESENTATION[item.status].label }}
          </Badge>
        </template>
      </RosterList>
    </template>

    <!-- 右侧：详情 + 面试记录 -->
    <template #detail>
      <EmptyState v-if="!selected" bare :icon="UsersRound" class="py-20">
        从左侧选择一位候选人查看详情
      </EmptyState>

      <template v-else>
        <CandidateDetailHeader
          :candidate="selected"
          :badge="{ label: STATUS_PRESENTATION[selected.status].label, variant: STATUS_PRESENTATION[selected.status].badge }"
        >
          <template #actions>
            <!-- 动作区可收缩换行（手机上两个按钮纵向堆叠），长房间名不再撑破资料卡 -->
            <div class="flex min-w-0 max-w-full flex-wrap justify-end gap-2">
              <Button
                v-if="canEditPreferences"
                size="sm"
                variant="outline"
                class="shrink-0"
                @click="openPreferences"
              >
                <Pencil aria-hidden="true" />
                改志愿
              </Button>
              <Button
                v-if="selected.room_id"
                size="sm"
                variant="outline"
                class="min-w-0 max-w-full"
                @click="goRoom(selected)"
              >
                <ExternalLink aria-hidden="true" />
                <span class="truncate">进入{{ roomLabelOf(selected.room_id) }}</span>
              </Button>
            </div>
          </template>

          <!-- 信息行：值样式与候选人管理 DataTable 单元格一致——数字类 tabular-nums（非等宽）、
               接受调剂纯文本、房间链接非等宽、创建时间 muted+nowrap。 -->
          <div class="space-y-2 pt-4 text-sm">
            <div class="flex items-center justify-between gap-3">
              <span class="text-muted-foreground">学号</span>
              <span class="whitespace-nowrap tabular-nums">{{ selected.student_no }}</span>
            </div>
            <div v-if="selected.first_choice" class="flex items-center justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">第一志愿</span>
              <span class="min-w-0 break-words text-right">{{ selected.first_choice }}</span>
            </div>
            <div v-if="selected.second_choice" class="flex items-center justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">第二志愿</span>
              <span class="min-w-0 break-words text-right">{{ selected.second_choice }}</span>
            </div>
            <div v-if="selected.first_choice || selected.second_choice" class="flex items-center justify-between gap-3">
              <span class="text-muted-foreground">接受调剂</span>
              <span class="whitespace-nowrap">{{ selected.accept_adjust ? '接受' : '不接受' }}</span>
            </div>
            <div v-if="selected.phone" class="flex items-center justify-between gap-3">
              <span class="text-muted-foreground">手机号</span>
              <span class="whitespace-nowrap tabular-nums">{{ selected.phone }}</span>
            </div>
            <div v-if="selected.qq" class="flex items-center justify-between gap-3">
              <span class="text-muted-foreground">QQ号</span>
              <span class="whitespace-nowrap tabular-nums">{{ selected.qq }}</span>
            </div>
            <div v-if="selected.email" class="flex items-center justify-between gap-3">
              <span class="text-muted-foreground">邮箱</span>
              <span class="break-all tabular-nums text-right">{{ selected.email }}</span>
            </div>
            <div class="flex items-center justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">房间</span>
              <span v-if="selected.room_id" class="flex min-w-0 flex-1 justify-end">
                <Button
                  variant="link"
                  class="h-auto min-w-0 max-w-full p-0 max-lg:min-h-11"
                  @click="goRoom(selected)"
                >
                  <span class="truncate">{{ roomLabelOf(selected.room_id) }}</span>
                </Button>
              </span>
              <span v-else class="text-muted-foreground">—</span>
            </div>
            <!-- 面试房间：面试结束那一刻记录（此时房间已与候选人解绑，故用当时的名字快照展示）。
                 没面完的候选人没有这条记录，整行不显示。 -->
            <div v-if="interviewRoomLabel" class="flex items-center justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">面试房间</span>
              <span class="min-w-0 break-words text-right">{{ interviewRoomLabel }}</span>
            </div>
            <div class="flex items-center justify-between gap-3">
              <span class="text-muted-foreground">创建时间</span>
              <time class="whitespace-nowrap text-muted-foreground">{{ formatDateTime(selected.created_at) }}</time>
            </div>
          </div>

          <!-- 录取决定：录取阶段展示。本部门用段式控件切换；跨部门浏览时补充其他部门决定 -->
          <template v-if="showControls">
            <div class="mt-4 space-y-3">
              <div v-if="canBrowseAll && othersOf.length" class="flex flex-wrap items-center gap-2">
                <Badge
                  v-for="d in othersOf"
                  :key="d.departmentId"
                  :variant="ADMISSION_PRESENTATION[d.status].badge"
                >
                  {{ d.departmentName }}：{{ ADMISSION_PRESENTATION[d.status].label }}
                </Badge>
              </div>

              <div
                v-if="canRecord && user?.department_id"
                class="inline-flex items-center rounded-lg bg-muted p-1"
                role="group"
                aria-label="本部门录取决定（快捷键 1/2/3）"
                title="快捷键：1 待定 / 2 录取 / 3 放弃"
              >
                <button
                  v-for="s in ADMISSION_STATUSES"
                  :key="s"
                  type="button"
                  :class="segmentedItemVariants({ active: ownStatus === s })"
                  :disabled="admissionsLoading"
                  @click="switchAdmission(selected, s)"
                >
                  {{ ADMISSION_PRESENTATION[s].label }}
                </button>
              </div>
              <span v-else-if="ownStatus" class="text-sm text-muted-foreground">
                {{ ADMISSION_PRESENTATION[ownStatus].label }}
              </span>
            </div>
          </template>
        </CandidateDetailHeader>

        <!-- 面试过程记录卡：主功能，留足空间 -->
        <Card>
          <CardHeader class="flex-row items-center justify-between space-y-0">
            <CardTitle class="text-sm">面试记录</CardTitle>
            <Badge v-if="!msgsLoading && !msgsError && messages.length" variant="outline">
              {{ messages.length }} 条
            </Badge>
          </CardHeader>
          <CardContent class="pt-0">
            <ListSkeleton v-if="msgsLoading" :rows="3" item-class="h-12 w-full rounded-xl" />
            <ErrorAlert v-else-if="msgsError" :message="msgsError" retry-label="重试" @retry="reloadMessages" />
            <p v-else-if="messages.length === 0" class="text-sm text-muted-foreground">
              暂无面试记录
            </p>
            <MessageTranscript v-else :messages="messages" @changed="reloadMessages" />

            <!-- 补充记录：归档写入（面试结档、房间解绑后仍可补），权限与房间聊天一致 -->
            <div v-if="canCompose" class="mt-4 border-t pt-3">
              <MessageComposer
                v-model="draft"
                :sending="msgsSending"
                placeholder="补充一条面试记录，Enter 发送…"
                label="补充面试记录"
                @send="submitMessage"
              />
            </div>
          </CardContent>
        </Card>

        <!-- 记录末尾：上一个 / 下一个候选人（↑/↓ 键盘可达） -->
        <RosterPager
          v-if="filtered.length > 0"
          :can-prev="canPrev"
          :can-next="canNext"
          @prev="goPrev"
          @next="goNext"
        />

        <!-- 志愿与调剂编辑（独立小权限） -->
        <CandidatePreferenceDialog
          :open="!!preferenceTarget"
          :candidate="preferenceTarget"
          @update:open="preferenceTarget = $event ? preferenceTarget : null"
          @saved="onPreferencesSaved"
        />
      </template>
    </template>
  </MasterDetailSplit>
</template>
