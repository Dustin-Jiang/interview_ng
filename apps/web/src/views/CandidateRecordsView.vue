<!--
  CandidateRecordsView —— 候选人查看页（与「候选人管理」分离，对普通用户开放）。
  左侧：候选人名册（搜索 + 状态筛选 + 键盘可达的 listbox）；右侧：选中候选人的详细资料与面试过程记录。
  移动端为「列表 ↔ 详情」两段式导航（选中候选人后进入详情，可返回列表）；桌面端左右分栏。
  筛选条件与选中条目分别同步到 URL query（`?q=`/`?status=`）与路径参数（`/candidates/:candidateId`，可深链 / 分享）；
  记录经 REST GET /api/candidates/:id/messages 拉取。

  组装层：布局与名册交给 MasterDetailSplit / RosterList / RosterPager，
  数据与交互交给 useCandidatePool / useRosterSelection / useCandidateMessages / useAdmissions。
-->
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ExternalLink, SearchX, SlidersHorizontal, UsersRound, X } from 'lucide-vue-next'

import { useAdmissions } from '@/composables/useAdmissions'
import { useAuth } from '@/composables/useAuth'
import { useBoardChannel } from '@/composables/useBoardChannel'
import { useCandidateMessages } from '@/composables/useCandidateMessages'
import { useCandidatePool } from '@/composables/useCandidatePool'
import { useDebouncedRefresh } from '@/composables/useDebouncedRefresh'
import { useRosterRouteSync } from '@/composables/useRosterRouteSync'
import { useRosterSelection } from '@/composables/useRosterSelection'
import { ADMISSION_PRESENTATION, STATUS_PRESENTATION } from '@/presenters/status'
import {
  ADMISSION_STATUSES,
  CANDIDATE_STATUSES,
  type Candidate,
  type CandidateStatus,
} from '@/models'
import { formatDateTime } from '@/lib/format'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { segmentedItemVariants } from '@/components/ui/tokens'
import CandidateDetailHeader from '@/components/app/CandidateDetailHeader.vue'
import EmptyState from '@/components/app/EmptyState.vue'
import ErrorAlert from '@/components/app/ErrorAlert.vue'
import ListSkeleton from '@/components/app/ListSkeleton.vue'
import MasterDetailSplit from '@/components/app/MasterDetailSplit.vue'
import MessageTranscript from '@/components/app/MessageTranscript.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'
import RosterList from '@/components/app/RosterList.vue'
import RosterPager from '@/components/app/RosterPager.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const route = useRoute()
const router = useRouter()
const { user } = useAuth()

// ---- 名册数据（一次拉取，客户端筛选） ----
const { candidates, loading: poolLoading, error: poolError, load } = useCandidatePool()

/** 搜索关键词（姓名 / 简介包含匹配，大小写不敏感）。 */
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
    return c.name.toLowerCase().includes(kw) || (c.profile ?? '').toLowerCase().includes(kw)
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
  highlight,
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
  extraKeys: onShortcutKey,
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

/** 全局快捷键：← / → 切换候选人；1/2/3 记录本部门录取决定。 */
function onShortcutKey(e: KeyboardEvent): boolean {
  const key = Number(e.key)
  if (!Number.isInteger(key) || key < 1 || key > ADMISSION_STATUSES.length) return false
  const candidate = selected.value
  if (!showControls.value || !canRecord.value || admissionsLoading.value) return true
  if (!candidate || !user.value?.department_id) return true
  void switchAdmission(candidate, ADMISSION_STATUSES[key - 1])
  return true
}

// ---- 面试过程记录 ----
const { messages, loading: msgsLoading, error: msgsError, load: loadMessages } = useCandidateMessages()

watch(selectedId, (id) => {
  if (id) void loadMessages(id)
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
const rosterReload = useDebouncedRefresh(() => void load())
useBoardChannel().subscribe((ev) => {
  if (BOARD_ROSTER_EVENTS.includes(ev.type)) return rosterReload.schedule()
  if (ev.type === 'message_appended') {
    const cid = (ev.data as { CandidateID?: number } | undefined)?.CandidateID
    if (cid != null && cid === selectedId.value) void loadMessages(cid)
  }
})

// ---- 深链与写回见上方（onMounted 恢复 + useUrlSync） ----

function roomLabel(c: Candidate): string {
  return c.room_id ? `#${c.room_id}` : '—'
}

function goRoom(c: Candidate) {
  if (!c.room_id) return
  void router.push({ name: 'room', params: { roomId: String(c.room_id) } })
}
</script>

<template>
  <MasterDetailSplit
    :show-detail="showDetail"
    aside-label="候选人名册"
    detail-label="候选人详情"
    back-aria-label="返回候选人列表"
    @back="closeDetail"
  >
    <!-- 左侧：名册工具条 + 名册 -->
    <template #aside>
      <!-- 紧凑工具条：标题 + 结果数 + 筛选浮层 + 刷新 -->
      <div class="flex items-center gap-2 p-3">
        <h1 class="flex min-w-0 items-center gap-2 truncate text-base font-semibold tracking-tight">
          <UsersRound class="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          候选人
        </h1>
        <span class="shrink-0 text-xs text-muted-foreground" aria-live="polite">
          {{ filtered.length }} / {{ candidates.length }}
        </span>
        <span class="ml-auto flex shrink-0 items-center gap-1">
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
            <PopoverContent class="w-72">
              <div class="space-y-3">
                <SearchInput v-model="keyword" full placeholder="搜索姓名 / 简介…" />
                <Select
                  :model-value="statusFilter || 'ALL'"
                  @update:model-value="statusFilter = $event === 'ALL' ? '' : ($event as CandidateStatus)"
                >
                  <SelectTrigger class="w-full" aria-label="按状态筛选">
                    <SelectValue placeholder="全部状态" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="ALL">全部状态</SelectItem>
                    <SelectItem v-for="s in CANDIDATE_STATUSES" :key="s" :value="s">
                      {{ STATUS_PRESENTATION[s].label }}
                    </SelectItem>
                  </SelectContent>
                </Select>
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
        </span>
      </div>

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
        @highlight="highlight"
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
        <template #meta="{ item }">
          <span v-if="item.room_id" class="shrink-0 font-mono">#{{ item.room_id }}</span>
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
            <Button
              v-if="selected.room_id"
              size="sm"
              variant="outline"
              class="shrink-0"
              @click="goRoom(selected)"
            >
              <ExternalLink aria-hidden="true" />
              进入房间 #{{ selected.room_id }}
            </Button>
          </template>

          <div class="space-y-2 pt-4 text-sm">
            <div class="flex items-center justify-between gap-3">
              <span class="text-muted-foreground">房间</span>
              <span v-if="selected.room_id" class="font-mono">
                <Button variant="link" class="h-auto p-0" @click="goRoom(selected)">
                  {{ roomLabel(selected) }}
                </Button>
              </span>
              <span v-else class="text-muted-foreground">{{ roomLabel(selected) }}</span>
            </div>
            <div class="flex items-center justify-between gap-3">
              <span class="text-muted-foreground">创建时间</span>
              <time>{{ formatDateTime(selected.created_at) }}</time>
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
            <MessageTranscript v-else :messages="messages" />
          </CardContent>
        </Card>

        <!-- 记录末尾：上一个 / 下一个候选人（←/→ 键盘可达） -->
        <RosterPager
          v-if="filtered.length > 0"
          :can-prev="canPrev"
          :can-next="canNext"
          @prev="goPrev"
          @next="goNext"
        />
      </template>
    </template>
  </MasterDetailSplit>
</template>
