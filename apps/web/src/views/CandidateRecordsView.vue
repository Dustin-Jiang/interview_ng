<!--
  CandidateRecordsView —— 候选人查看页（与「候选人管理」分离，对普通用户开放）。
  左侧：候选人名册（搜索 + 状态筛选 + 键盘可达的 listbox）；右侧：选中候选人的详细资料与面试过程记录。
  移动端为「列表 ↔ 详情」两段式导航（选中候选人后进入详情，可返回列表）；桌面端左右分栏。
  筛选条件与选中候选人同步到 URL query（可深链 / 分享）；记录经 REST GET /api/candidates/:id/messages 拉取。
-->
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, ExternalLink, RefreshCw, SearchX, UsersRound, X } from 'lucide-vue-next'

import { candidateApi } from '@/api/http'
import { formatDateTime } from '@/lib/format'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { CANDIDATE_STATUSES, PERMISSIONS, type Candidate, type Message } from '@/models'
import { useAuth } from '@/composables/useAuth'
import { cn } from '@/lib/utils'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Separator } from '@/components/ui/separator'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Avatar, AvatarFallback, avatarVariants } from '@/components/ui/avatar'
import EmptyState from '@/components/app/EmptyState.vue'
import MessageTranscript from '@/components/app/MessageTranscript.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const router = useRouter()
const route = useRoute()
const { hasPermission } = useAuth()

// ---- 名册数据（一次拉取，客户端筛选） ----
const candidates = ref<Candidate[]>([])
const loading = ref(false)
const error = ref('')

/** 搜索关键词（姓名 / 简介包含匹配，大小写不敏感）。 */
const keyword = ref('')
/** 状态筛选：'' = 全部。 */
const statusFilter = ref<'' | (typeof CANDIDATE_STATUSES)[number]>('')
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

onBeforeUnmount(() => clearTimeout(syncTimer))

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
  <div class="flex h-full flex-col overflow-hidden lg:flex-row">
    <!-- 左侧：候选人名册 -->
    <aside
      :class="cn(asideClass, 'h-full min-w-0 shrink-0 flex-col border-b lg:w-80 lg:border-b-0 lg:border-r')"
      aria-label="候选人名册"
    >
      <div class="space-y-3 border-b p-4">
        <div class="flex items-center justify-between gap-2">
          <h1 class="flex items-center gap-2 text-base font-semibold tracking-tight">
            <UsersRound class="h-4 w-4 text-muted-foreground" aria-hidden="true" />
            候选人
          </h1>
          <Button variant="ghost" size="icon" aria-label="刷新列表" :disabled="loading" @click="load">
            <RefreshCw :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
          </Button>
        </div>
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
      </div>

      <!-- 结果统计条 -->
      <div class="flex items-center justify-between gap-2 border-b px-4 py-2 text-xs text-muted-foreground">
        <span aria-live="polite">
          {{ hasFilter ? `筛选出 ${filtered.length} / ${candidates.length} 位` : `共 ${candidates.length} 位候选人` }}
        </span>
        <button
          v-if="hasFilter"
          type="button"
          class="inline-flex items-center gap-1 rounded-sm transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-label="清除筛选"
          @click="clearFilters"
        >
          <X class="h-3.5 w-3.5" aria-hidden="true" />
          清除筛选
        </button>
      </div>

      <!-- 名册滚动区 -->
      <ScrollArea class="min-h-0 flex-1">
        <div ref="rosterEl" role="listbox" aria-label="候选人列表" class="p-2" @keydown="onRosterKeydown">
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
            <template v-if="hasFilter" #hint>试试调整关键词或状态筛选</template>
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
                <Badge :variant="STATUS_PRESENTATION[c.status].badge" class="shrink-0">
                  {{ STATUS_PRESENTATION[c.status].label }}
                </Badge>
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

    <!-- 右侧：详情 + 面试记录 -->
    <section
      :class="cn(detailClass, 'h-full min-w-0 flex-1 flex-col')"
      aria-label="候选人详情"
    >
      <!-- 移动端返回栏 -->
      <div class="flex items-center gap-2 border-b px-4 py-2 lg:hidden">
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
            <template #hint>含个人资料与完整面试过程记录</template>
          </EmptyState>

          <template v-else>
            <!-- 资料卡 -->
            <Card>
              <CardHeader class="flex-row items-center gap-4 space-y-0">
                <Avatar :class="avatarVariants({ size: 'lg' })" aria-hidden="true">
                  <AvatarFallback>{{ initialsOf(selectedCandidate.name) }}</AvatarFallback>
                </Avatar>
                <div class="min-w-0 flex-1 space-y-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <CardTitle class="truncate text-xl">{{ selectedCandidate.name }}</CardTitle>
                    <Badge :variant="STATUS_PRESENTATION[selectedCandidate.status].badge">
                      {{ STATUS_PRESENTATION[selectedCandidate.status].label }}
                    </Badge>
                  </div>
                  <CardDescription class="line-clamp-2">
                    {{ selectedCandidate.profile || '暂无个人简介' }}
                  </CardDescription>
                </div>
                <!-- 进行中的面试可直达房间 -->
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
                <Separator class="mb-4" />
                <dl class="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
                  <div class="flex items-baseline gap-2">
                    <dt class="shrink-0 text-muted-foreground">房间</dt>
                    <dd v-if="selectedCandidate.room_id" class="font-mono">
                      <Button variant="link" class="h-auto p-0" @click="goRoom(selectedCandidate)">
                        {{ roomLabel(selectedCandidate) }}
                      </Button>
                    </dd>
                    <dd v-else class="text-muted-foreground">{{ roomLabel(selectedCandidate) }}</dd>
                  </div>
                  <div class="flex items-baseline gap-2">
                    <dt class="shrink-0 text-muted-foreground">创建时间</dt>
                    <dd>{{ formatDateTime(selectedCandidate.created_at) }}</dd>
                  </div>
                </dl>
              </CardContent>
            </Card>

            <!-- 面试过程记录 -->
            <Card>
              <CardHeader class="flex-row items-center justify-between space-y-0">
                <CardTitle class="text-sm">面试记录</CardTitle>
                <Badge v-if="!msgsLoading && !msgsError && messages.length" variant="outline">
                  {{ messages.length }} 条
                </Badge>
              </CardHeader>
              <CardContent>
                <div v-if="msgsLoading" class="space-y-2" aria-busy="true">
                  <Skeleton v-for="i in 3" :key="i" class="h-12 w-full rounded-xl" />
                </div>

                <div v-else-if="msgsError" role="alert" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
                  {{ msgsError }}
                  <Button variant="link" class="h-auto p-0" @click="selectedId && loadMessages(selectedId)">重试</Button>
                </div>

                <p v-else-if="messages.length === 0" class="text-sm text-muted-foreground">
                  暂无面试记录{{ selectedCandidate.status !== 'COMPLETED' ? '（面试进行中可在房间内实时记录）' : '' }}
                </p>

                <MessageTranscript v-else :messages="messages" />

                <!-- 权限提示：管理动作在候选人管理页 -->
                <p v-if="hasPermission(PERMISSIONS.CANDIDATES_MANAGE)" class="pt-4 text-xs text-muted-foreground">
                  需要{{ selectedCandidate.status === 'NOT_CHECKED_IN' ? '签到' : '编辑 / 重置' }}？前往「候选人管理」。
                </p>
              </CardContent>
            </Card>
          </template>
        </div>
      </ScrollArea>
    </section>
  </div>
</template>
