<!--
  CandidateRecordsView —— 候选人查看页（与「候选人管理」分离，对普通用户开放）。
  左侧：候选人名册（搜索 + 状态筛选 + 键盘可达的 listbox）；右侧：选中候选人的详细资料与面试过程记录。
  移动端为「列表 ↔ 详情」两段式导航（选中候选人后进入详情，可返回列表）；桌面端左右分栏。
  筛选条件与选中候选人同步到 URL query（可深链 / 分享）；记录经 REST GET /api/candidates/:id/messages 拉取。
-->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { ArrowLeft, ExternalLink, RefreshCw, SearchX, SlidersHorizontal, UsersRound, X } from 'lucide-vue-next'

import { admissionApi, candidateApi, departmentApi, systemStatusApi } from '@/api/http'
import { useBoardChannel } from '@/composables/useBoardChannel'
import { formatDateTime } from '@/lib/format'
import { ADMISSION_PRESENTATION, STATUS_PRESENTATION } from '@/presenters/status'
import { CANDIDATE_STATUSES, PERMISSIONS, ADMISSION_STATUSES, type AdmissionStatus, type Candidate, type CandidateAdmission, type Message, type SystemStatus } from '@/models'
import { cn } from '@/lib/utils'
import { useAuth } from '@/composables/useAuth'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Avatar, AvatarFallback, avatarVariants } from '@/components/ui/avatar'
import { segmentedItemVariants } from '@/components/ui/tokens'
import EmptyState from '@/components/app/EmptyState.vue'
import MessageTranscript from '@/components/app/MessageTranscript.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const router = useRouter()
const route = useRoute()

// ---- 名册数据（一次拉取，客户端筛选） ----
const candidates = ref<Candidate[]>([])
const loading = ref(false)
const error = ref('')

/** 搜索关键词（姓名 / 简介包含匹配，大小写不敏感）。 */
const keyword = ref('')
/** 状态筛选：'' = 全部。 */
const statusFilter = ref<'' | (typeof CANDIDATE_STATUSES)[number]>('')
/** 筛选浮层开关。 */
const filterOpen = ref(false)
/** 移动端：列表 / 详情两段式切换（桌面端恒为分栏，该值不影响 lg 以上布局）。 */
const showDetail = ref(false)

/** 当前选中候选人 id（名册高亮 + 右侧详情）。 */
const selectedId = ref<number | null>(null)

/** 深链待选候选人 id：列表加载完成后若在名册内则选中。 */
let pendingCandidateId: number | undefined

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

/** 姓名首字母（头像回退位）。 */
function initialsOf(name: string): string {
  const trimmed = name.trim()
  const parts = trimmed.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return (trimmed.slice(0, 2) || '?').toUpperCase()
}

function clearFilters() {
  keyword.value = ''
  statusFilter.value = ''
}

async function load() {
  if (loading.value) return
  loading.value = true
  error.value = ''
  try {
    const res = await candidateApi.list({ limit: 200 })
    candidates.value = res.items
    // 深链候选人在当前列表内则选中它；否则默认首位，避免右侧空转。
    if (pendingCandidateId && res.items.some((c) => c.id === pendingCandidateId)) {
      selectedId.value = pendingCandidateId
      showDetail.value = true
    } else if (!selectedId.value && res.items.length) {
      selectedId.value = res.items[0].id
    }
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
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
let rosterTimer: ReturnType<typeof setTimeout> | undefined
useBoardChannel().subscribe((ev) => {
  if (BOARD_ROSTER_EVENTS.includes(ev.type)) {
    clearTimeout(rosterTimer)
    rosterTimer = setTimeout(() => void load(), 300)
    return
  }
  if (ev.type === 'message_appended') {
    const cid = (ev.data as { CandidateID?: number } | undefined)?.CandidateID
    if (cid != null && cid === selectedId.value) void loadMessages(cid)
  }
})

onMounted(() => {
  // 从 URL query 恢复筛选与选中态（深链）。
  const q = route.query.q
  const st = route.query.status
  const cand = route.query.candidate
  if (typeof q === 'string') keyword.value = q
  if (typeof st === 'string' && (CANDIDATE_STATUSES as readonly string[]).includes(st)) {
    statusFilter.value = st as (typeof CANDIDATE_STATUSES)[number]
  }
  if (typeof cand === 'string' && /^\d+$/.test(cand)) {
    pendingCandidateId = Number(cand)
    showDetail.value = true
  }
  void load()
})

// ---- 深链同步：筛选 / 选中变更 → 写回 URL（replace，不污染历史） ----
let syncTimer: ReturnType<typeof setTimeout> | undefined

function syncUrl() {
  clearTimeout(syncTimer)
  syncTimer = setTimeout(() => {
    const query: Record<string, string> = {}
    const kw = keyword.value.trim()
    if (kw) query.q = kw
    if (statusFilter.value) query.status = statusFilter.value
    if (selectedId.value) query.candidate = String(selectedId.value)
    void router.replace({ query })
  }, 300)
}

watch([keyword, statusFilter, selectedId], syncUrl)

onBeforeUnmount(() => {
  clearTimeout(syncTimer)
  clearTimeout(rosterTimer)
})

// ---- 右侧详情 ----
const selectedCandidate = computed(() => candidates.value.find((c) => c.id === selectedId.value) ?? null)

const messages = ref<Message[]>([])
const msgsLoading = ref(false)
const msgsError = ref('')

async function loadMessages(id: number) {
  messages.value = []
  msgsError.value = ''
  msgsLoading.value = true
  try {
    const res = await candidateApi.messages(id)
    if (selectedId.value === id) messages.value = res.items // 过期响应丢弃
  } catch (e) {
    if (selectedId.value === id) msgsError.value = (e as Error).message
  } finally {
    if (selectedId.value === id) msgsLoading.value = false
  }
}

watch(selectedId, (id) => {
  if (id) void loadMessages(id)
})

function select(c: Candidate) {
  selectedId.value = c.id
  showDetail.value = true
}

// ---- 录取决定（录取阶段展示；按部门分别记录） ----
const { hasPermission, user } = useAuth()

/** 系统是否处于录取阶段（录取阶段才展示录取状态控件）。 */
const systemPhase = ref<SystemStatus['phase'] | null>(null)
const phaseLoading = ref(false)

/** 当前用户可见的录取决定（默认仅本部门，后端按权限过滤）。 */
const admissions = ref<CandidateAdmission[]>([])
const admissionsLoading = ref(false)
/** 本部门对候选人的录取决定集（candidate_id -> status 展示用）。 */
const admissionByCandidate = computed(() => {
  const mine = user.value?.department_id
  if (!mine) return new Map<number, AdmissionStatus>()
  const map = new Map<number, AdmissionStatus>()
  for (const a of admissions.value) {
    if (a.department_id !== mine) continue
    map.set(a.candidate_id, a.status)
  }
  return map
})

/** 是否持跨部门查看权限（决定录取决定是否全员可见）。 */
const canBrowseAllAdmissions = computed(() => hasPermission(PERMISSIONS.CANDIDATES_BROWSE_ALL))
/** 是否可记录录取决定（candidates.manage）。 */
const canRecordAdmission = computed(() => hasPermission(PERMISSIONS.CANDIDATES_MANAGE))

/** 部门名（跨部门查看时展示各部门决定归属）。 */
const departmentNames = ref(new Map<number, string>())

/** 当前选中候选人的所有可见录取决定（browse_all 跨部门时为多部门记录）。 */
const selectedAdmissions = computed(() =>
  admissions.value.filter((a) => a.candidate_id === selectedCandidate.value?.id),
)

/** 当前选中候选人本部门的录取决定（记录/切换按钮的激活态）。 */
const ownAdmissionStatus = computed<AdmissionStatus | undefined>(() => {
  const mine = user.value?.department_id
  const c = selectedCandidate.value
  if (!mine || !c) return undefined
  return admissionByCandidate.value.get(c.id)
})

/** 其他部门录取决定（跨部门浏览时，排除本部门后的其余记录）。 */
const otherDeptAdmissions = computed(() => {
  const mine = user.value?.department_id
  return selectedAdmissions.value.filter((a) => a.department_id !== mine)
})

/** 所有部门录取决定（跨部门浏览时含本部门；未记录默认待定）。 */
const allDeptAdmissions = computed(() => {
  if (!canBrowseAllAdmissions.value) return []
  const mine = user.value?.department_id
  const result: { departmentId: number; departmentName: string; status: AdmissionStatus }[] = []
  for (const [deptId, deptName] of departmentNames.value) {
    if (deptId === mine) continue
    const admission = selectedAdmissions.value.find((a) => a.department_id === deptId)
    result.push({
      departmentId: deptId,
      departmentName: deptName,
      status: admission?.status ?? 'pending',
    })
  }
  return result
})

/** 是否展示录取状态控件：录取阶段 + 有可看内容（本部门记录或跨部门权限）。 */
const showAdmissionControls = computed(() => {
  if (systemPhase.value !== 'admission') return false
  if (canBrowseAllAdmissions.value) return true
  return !!user.value?.department_id
})

async function loadSystemPhase() {
  if (phaseLoading.value) return
  phaseLoading.value = true
  try {
    const s = await systemStatusApi.get()
    systemPhase.value = s.phase
  } catch {
    systemPhase.value = null
  } finally {
    phaseLoading.value = false
  }
}

async function loadAdmissions() {
  if (!showAdmissionControls.value) return
  if (admissionsLoading.value) return
  admissionsLoading.value = true
  try {
    const res = await admissionApi.list()
    admissions.value = res.items ?? []
    if (canBrowseAllAdmissions.value) {
      const depts = await departmentApi.list()
      departmentNames.value = new Map(depts.items.map((d) => [d.id, d.name]))
    }
  } catch {
    admissions.value = []
  } finally {
    admissionsLoading.value = false
  }
}

/** 切换候选人录取决定（写入本部门记录）。 */
async function switchAdmission(c: Candidate, status: AdmissionStatus) {
  if (admissionsLoading.value) return
  try {
    await admissionApi.set(c.id, status)
    toast.success(`已将 ${c.name} 标记为「${ADMISSION_PRESENTATION[status].label}」`)
    await loadAdmissions()
  } catch (e) {
    toast.error((e as Error).message)
  }
}

watch(showAdmissionControls, (on) => {
  if (on) void loadAdmissions()
})

onMounted(() => {
  void loadSystemPhase()
})

// ---- 名册键盘导航：roving tabindex，仅选中项可 Tab，方向键在列表内移动 ----
const rosterEl = ref<HTMLElement | null>(null)

function onRosterKeydown(e: KeyboardEvent) {
  if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
  e.preventDefault()
  const list = filtered.value
  const idx = list.findIndex((c) => c.id === selectedId.value)
  if (idx === -1) return
  const dir = e.key === 'ArrowDown' ? 1 : -1
  const next = list[idx + dir]
  if (!next) return
  selectedId.value = next.id
  void nextTick(() => {
    const btns = rosterEl.value?.querySelectorAll<HTMLElement>('[role="option"]')
    const i = list.findIndex((c) => c.id === next.id)
    btns?.[i]?.focus()
  })
}

function roomLabel(c: Candidate): string {
  return c.room_id ? `#${c.room_id}` : '—'
}

function goRoom(c: Candidate) {
  if (!c.room_id) return
  void router.push({ name: 'room', params: { roomId: String(c.room_id) } })
}

const asideClass = computed(() => (showDetail.value ? 'hidden lg:flex' : 'flex'))
const detailClass = computed(() => (showDetail.value ? 'flex' : 'hidden lg:flex'))
</script>

<template>
  <!-- 整体居中并限制最大宽度（max-w-content 语义档位），左右分栏 -->
  <div class="mx-auto flex h-full w-full max-w-content overflow-hidden">
    <!-- 左侧：候选人名册（无边框、底色与右侧一致） -->
    <aside
      :class="cn(asideClass, 'h-full min-w-0 shrink-0 flex-col lg:w-80')"
      aria-label="候选人名册"
    >
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
                  @update:model-value="statusFilter = $event === 'ALL' ? '' : ($event as any)"
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
          <Button variant="ghost" size="icon" class="h-8 w-8" aria-label="刷新列表" :disabled="loading" @click="load">
            <RefreshCw :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
          </Button>
        </span>
      </div>

      <!-- 名册滚动区 -->
      <ScrollArea class="min-h-0 flex-1">
        <div ref="rosterEl" role="listbox" aria-label="候选人列表" class="space-y-1 p-2" @keydown="onRosterKeydown">
          <!-- 加载骨架屏 -->
          <div v-if="loading && candidates.length === 0" class="space-y-2 p-2" aria-busy="true">
            <Skeleton v-for="i in 5" :key="i" class="h-14 w-full rounded-md" />
          </div>

          <!-- 拉取失败 -->
          <div v-else-if="error" role="alert" class="p-4 text-sm text-destructive">
            {{ error }}
            <Button variant="link" class="h-auto p-0" @click="load">重试</Button>
          </div>

          <!-- 空态 -->
          <EmptyState v-else-if="filtered.length === 0" bare :icon="hasFilter ? SearchX : UsersRound" class="py-10">
            {{ hasFilter ? '没有匹配的候选人' : '暂无候选人' }}
            <template v-if="hasFilter" #action>
              <Button variant="outline" size="sm" @click="clearFilters">清除筛选</Button>
            </template>
          </EmptyState>

          <!-- 列表项：整行可点、键盘可达（roving tabindex + 方向键） -->
          <button
            v-for="c in filtered"
            :key="c.id"
            type="button"
            role="option"
            :aria-selected="c.id === selectedId"
            :tabindex="c.id === selectedId ? 0 : -1"
            class="flex w-full cursor-pointer items-center gap-3 rounded-md p-3 text-left transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            :class="c.id === selectedId ? 'bg-accent' : ''"
            @click="select(c)"
          >
            <Avatar :class="avatarVariants({ size: 'sm' })" aria-hidden="true">
              <AvatarFallback>{{ initialsOf(c.name) }}</AvatarFallback>
            </Avatar>
            <span class="min-w-0 flex-1">
              <span class="flex items-center justify-between gap-2">
                <span class="min-w-0 truncate text-sm font-medium">{{ c.name }}</span>
                <span class="flex shrink-0 items-center gap-1">
                  <Badge :variant="STATUS_PRESENTATION[c.status].badge">
                    {{ STATUS_PRESENTATION[c.status].label }}
                  </Badge>
                  <Badge
                    v-if="showAdmissionControls && admissionByCandidate.get(c.id)"
                    :variant="ADMISSION_PRESENTATION[admissionByCandidate.get(c.id)!].badge"
                  >
                    {{ ADMISSION_PRESENTATION[admissionByCandidate.get(c.id)!].label }}
                  </Badge>
                </span>
              </span>
              <span class="mt-0.5 flex items-center justify-between gap-2 text-xs text-muted-foreground">
                <span class="min-w-0 truncate">{{ c.profile || '无简介' }}</span>
                <span v-if="c.room_id" class="shrink-0 font-mono">#{{ c.room_id }}</span>
              </span>
            </span>
          </button>
        </div>
      </ScrollArea>
    </aside>

    <!-- 右侧：详情 + 面试记录（无边框、留白给主功能） -->
    <section
      :class="cn(detailClass, 'h-full min-w-0 flex-1 flex-col')"
      aria-label="候选人详情"
    >
      <!-- 移动端返回栏 -->
      <div class="flex items-center gap-2 px-4 py-2 lg:hidden">
        <Button variant="ghost" size="sm" aria-label="返回候选人列表" @click="showDetail = false">
          <ArrowLeft aria-hidden="true" />
          返回列表
        </Button>
      </div>

      <ScrollArea class="min-h-0 flex-1">
        <div class="mx-auto max-w-3xl space-y-6 px-4 py-6">
          <!-- 未选择 -->
          <EmptyState v-if="!selectedCandidate" bare :icon="UsersRound" class="py-20">
            从左侧选择一位候选人查看详情
          </EmptyState>

          <template v-else>
            <!-- 资料卡：头像 + 姓名 + 状态 + 直达房间 + 元信息 -->
            <Card>
              <CardHeader class="flex-row items-start gap-4 space-y-0">
                <Avatar :class="avatarVariants({ size: 'lg' })" aria-hidden="true">
                  <AvatarFallback>{{ initialsOf(selectedCandidate.name) }}</AvatarFallback>
                </Avatar>
                <div class="min-w-0 flex-1 space-y-1.5">
                  <div class="flex flex-wrap items-center gap-2">
                    <CardTitle class="truncate text-xl">{{ selectedCandidate.name }}</CardTitle>
                    <Badge :variant="STATUS_PRESENTATION[selectedCandidate.status].badge">
                      {{ STATUS_PRESENTATION[selectedCandidate.status].label }}
                    </Badge>
                  </div>
                  <p class="text-sm text-muted-foreground">
                    {{ selectedCandidate.profile || '暂无个人简介' }}
                  </p>
                </div>
                <Button
                  v-if="selectedCandidate.room_id"
                  size="sm"
                  variant="outline"
                  class="shrink-0"
                  @click="goRoom(selectedCandidate)"
                >
                  <ExternalLink aria-hidden="true" />
                  进入房间 #{{ selectedCandidate.room_id }}
                </Button>
              </CardHeader>
              <CardContent class="pt-0">
                <div class="space-y-2 pt-4 text-sm">
                  <div class="flex items-center justify-between gap-3">
                    <span class="text-muted-foreground">房间</span>
                    <span v-if="selectedCandidate.room_id" class="font-mono">
                      <Button variant="link" class="h-auto p-0" @click="goRoom(selectedCandidate)">
                        {{ roomLabel(selectedCandidate) }}
                      </Button>
                    </span>
                    <span v-else class="text-muted-foreground">{{ roomLabel(selectedCandidate) }}</span>
                  </div>
                  <div class="flex items-center justify-between gap-3">
                    <span class="text-muted-foreground">创建时间</span>
                    <time>{{ formatDateTime(selectedCandidate.created_at) }}</time>
                  </div>
                </div>

                <!-- 录取决定：录取阶段展示。本部门用段式控件切换；跨部门浏览时补充其他部门决定 -->
                <template v-if="showAdmissionControls">
                  <div class="mt-4 space-y-3">
                    <div v-if="canBrowseAllAdmissions && allDeptAdmissions.length" class="flex flex-wrap items-center gap-2">
                      <Badge
                        v-for="d in allDeptAdmissions"
                        :key="d.departmentId"
                        :variant="ADMISSION_PRESENTATION[d.status].badge"
                      >
                        {{ d.departmentName }}：{{ ADMISSION_PRESENTATION[d.status].label }}
                      </Badge>
                    </div>

                    <div
                      v-if="canRecordAdmission && user?.department_id"
                      class="inline-flex items-center rounded-lg bg-muted p-1"
                      role="group"
                      aria-label="本部门录取决定"
                    >
                      <button
                        v-for="s in ADMISSION_STATUSES"
                        :key="s"
                        type="button"
                        :class="segmentedItemVariants({ active: ownAdmissionStatus === s })"
                        :disabled="admissionsLoading"
                        @click="switchAdmission(selectedCandidate, s)"
                      >
                        {{ ADMISSION_PRESENTATION[s].label }}
                      </button>
                    </div>
                    <span v-else-if="ownAdmissionStatus" class="text-sm text-muted-foreground">
                      {{ ADMISSION_PRESENTATION[ownAdmissionStatus].label }}
                    </span>
                  </div>
                </template>
              </CardContent>
            </Card>

            <!-- 面试过程记录卡：主功能，留足空间 -->
            <Card>
              <CardHeader class="flex-row items-center justify-between space-y-0">
                <CardTitle class="text-sm">面试记录</CardTitle>
                <Badge v-if="!msgsLoading && !msgsError && messages.length" variant="outline">
                  {{ messages.length }} 条
                </Badge>
              </CardHeader>
              <CardContent class="pt-0">
                <div v-if="msgsLoading" class="space-y-2" aria-busy="true">
                  <Skeleton v-for="i in 3" :key="i" class="h-12 w-full rounded-xl" />
                </div>

                <div v-else-if="msgsError" role="alert" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
                  {{ msgsError }}
                  <Button variant="link" class="h-auto p-0" @click="selectedId && loadMessages(selectedId)">重试</Button>
                </div>

                <p v-else-if="messages.length === 0" class="text-sm text-muted-foreground">
                  暂无面试记录
                </p>

                <MessageTranscript v-else :messages="messages" />
              </CardContent>
            </Card>
          </template>
        </div>
      </ScrollArea>
    </section>
  </div>
</template>
