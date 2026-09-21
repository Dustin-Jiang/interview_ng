<!--
  LeftoverView —— 捡漏竞拍（布局与 CandidateRecordsView 一致：左名册 + 右详情，移动端两段式）。
  左侧：COMPLETED 候选人名册（搜索 + 刷新 + 键盘可达的 listbox）；
  右侧：预算卡（本部门 / browse_all 各部门）+ 选中候选人的出价与结算详情。
  出价需 admissions.record 且已分配部门且处于捡漏阶段（行内数字输入 + Enter/按钮保存）；
  结算按钮仅对持 candidates.manage 的用户且处于捡漏阶段出现（ConfirmDialog 确认，
  赢家为最高出价部门）；结算结果以带文字 Badge 呈现，未正式落库时回退展示
  GET /leftover/final 的计算结果。结算阶段竞拍数据只读。无部门用户只读。
-->
<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ArrowLeft, ChevronLeft, ChevronRight, Check, Eye, EyeOff, Gavel, RefreshCw, SearchX, UsersRound } from 'lucide-vue-next'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { candidateApi, leftoverApi, systemStatusApi } from '@/api/http'
import { useAsync } from '@/composables/useAsync'
import { useAuth } from '@/composables/useAuth'
import { useBoardRefresh } from '@/composables/useBoardChannel'
import { PERMISSIONS, type Bid, type Candidate, type SystemPhase } from '@/models'
import { cn } from '@/lib/utils'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Avatar, AvatarFallback, avatarVariants } from '@/components/ui/avatar'
import SearchInput from '@/components/app/SearchInput.vue'
import { NumberField, NumberFieldContent, NumberFieldDecrement, NumberFieldIncrement, NumberFieldInput } from '@/components/ui/number-field'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import EmptyState from '@/components/app/EmptyState.vue'

const router = useRouter()
const route = useRoute()
const { hasPermission, user } = useAuth()

// ---- 数据加载：总览 / 本部门出价 / 结算结果 / 最终结果 / 候选人池（各自独立异步资源） ----
const {
  data: overviewData,
  loading: overviewLoading,
  error: overviewError,
  run: loadOverview,
} = useAsync(() => leftoverApi.overview())
const { data: bidsData, loading: bidsLoading, run: loadBids } = useAsync(() => leftoverApi.bids())
const {
  data: resultsData,
  loading: resultsLoading,
  run: loadResults,
} = useAsync(() => leftoverApi.results())
const {
  data: poolData,
  loading: poolLoading,
  error: poolError,
  run: loadPool,
} = useAsync(() => candidateApi.list({ limit: 200 }))

const overview = computed(() => overviewData.value)
/** 候选人池：录取阶段的候选人（待录取 / 已录取 / 面试已结束兜底）。 */
const ADMISSION_POOL_STATUSES: ReadonlySet<string> = new Set([
  'ADMISSION_PENDING',
  'ADMITTED',
  'COMPLETED',
])
const candidates = computed<readonly Candidate[]>(() =>
  (poolData.value?.items ?? []).filter((c) => ADMISSION_POOL_STATUSES.has(c.status)),
)
/** 出价按候选人索引：管理员（browse_all）响应含全部门出价，编辑列仅取本部门（无部门则空）。 */
const bidsByCandidate = computed(() => {
  const items = bidsData.value?.items ?? []
  const mine = canBrowseAll.value
    ? items.filter((b) => b.department_id === overview.value?.my?.department_id)
    : items
  return new Map(mine.map((b) => [b.candidate_id, b]))
})
/** 管理端视图：候选人 → 全部门出价（按金额降序）。 */
const allBidsByCandidate = computed(() => {
  const map = new Map<number, Bid[]>()
  for (const b of bidsData.value?.items ?? []) {
    const list = map.get(b.candidate_id) ?? []
    list.push(b)
    map.set(b.candidate_id, list)
  }
  for (const list of map.values()) list.sort((x, y) => y.amount - x.amount)
  return map
})
/** 已成交结果按候选人索引（GET /leftover/results，唯一录取确定才返回）。 */
const resultsByCandidate = computed(
  () => new Map((resultsData.value?.items ?? []).map((r) => [r.candidate_id, r])),
)
/** 部门 id → 名称（结算结果 Badge 用；部门名单来自总览接口）。 */
const deptNameById = computed(
  () => new Map((overview.value?.departments ?? []).map((d) => [d.id, d.name])),
)
const loading = computed(
  () => bidsLoading.value || resultsLoading.value || poolLoading.value,
)
const error = computed(() => poolError.value || overviewError.value || '')

/** 整页重拉：出价/结算事件与手动刷新共用同一组加载。 */
async function reloadAll(): Promise<void> {
  await Promise.all([loadOverview(), loadBids(), loadResults(), loadPool()])
}

useBoardRefresh(['leftover_bid', 'leftover_resolved'], () => void reloadAll())

// ---- 出价步长：来自系统状态（管理面板可设，默认 10）；上下键由 number input 原生按 step 步进 ----
const bidStep = ref(10)
watch(overview, async (ov) => {
  if (ov && bidStep.value === 10) {
    try {
      const st = await systemStatusApi.get()
      bidStep.value = st.bid_step > 0 ? st.bid_step : 10
    } catch {
      /* 保底默认 10 */
    }
  }
})

// ---- 阶段与权限 ----
const PHASE_LABELS: Record<SystemPhase, string> = {
  interview: '面试阶段',
  admission: '录取阶段',
  leftover: '捡漏阶段',
  settlement: '结算阶段',
}
const phaseLabel = computed(() => {
  const phase = overview.value?.phase
  return phase ? PHASE_LABELS[phase] : '-'
})
const isLeftoverPhase = computed(() => overview.value?.phase === 'leftover')
const canManage = computed(() => hasPermission(PERMISSIONS.CANDIDATES_MANAGE))
/** 管理端跨部门浏览（candidates.browse_all）：可见全部门出价与各部门预算占用。 */
const canBrowseAll = computed(() => hasPermission(PERMISSIONS.CANDIDATES_BROWSE_ALL))
/** 可出价：持 admissions.record + 已分配部门 + 处于捡漏阶段（与后端前置校验一致）。 */
const canBid = computed(
  () => hasPermission(PERMISSIONS.ADMISSIONS_RECORD) && !!overview.value?.my && isLeftoverPhase.value,
)

// ---- 名册：搜索 + 已录取筛选 + 选中（深链 ?candidate= 同步到 URL query） ----
const keyword = ref('')
const hasFilter = computed(() => Boolean(keyword.value.trim()))
/** 已录取（唯一录取确定、封盘）的候选人默认从名册滤除，可切换显示。 */
const showSettled = ref(false)
const settledIds = computed(() => new Set(resultsByCandidate.value.keys()))
const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return candidates.value.filter((c) => {
    if (!showSettled.value && settledIds.value.has(c.id)) return false
    if (!kw) return true
    return c.name.toLowerCase().includes(kw)
  })
})

const selectedId = ref<number | null>(null)
let pendingCandidateId: number | undefined

const selectedCandidate = computed(() => candidates.value.find((c) => c.id === selectedId.value) ?? null)

/** 候选人被当前筛选隐藏时回退选中首位可见项，避免右侧停留在不可见候选人。 */
watch(filtered, (list) => {
  if (list.length && !list.some((c) => c.id === selectedId.value)) {
    selectedId.value = list[0].id
  }
})
/** 姓名首字母（头像回退位）。 */
function initialsOf(name: string): string {
  const trimmed = name.trim()
  const parts = trimmed.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return (trimmed.slice(0, 2) || '?').toUpperCase()
}

watch(candidates, (list) => {
  if (pendingCandidateId && list.some((c) => c.id === pendingCandidateId)) {
    selectedId.value = pendingCandidateId
    showDetail.value = true
    pendingCandidateId = undefined
  } else if (!selectedId.value || !list.some((c) => c.id === selectedId.value)) {
    selectedId.value = list[0]?.id ?? null
  }
})

function select(c: Candidate) {
  selectedId.value = c.id
  showDetail.value = true
}

// ---- 深链同步：选中变更 → 写回 URL query（replace，不污染历史） ----
let syncTimer: ReturnType<typeof setTimeout> | undefined

function syncUrl() {
  clearTimeout(syncTimer)
  syncTimer = setTimeout(() => {
    const query: Record<string, string> = {}
    const kw = keyword.value.trim()
    if (kw) query.q = kw
    if (selectedId.value) query.candidate = String(selectedId.value)
    void router.replace({ query })
  }, 300)
}

watch([keyword, selectedId], syncUrl)

onMounted(() => {
  const q = route.query.q
  const cand = route.query.candidate
  if (typeof q === 'string') keyword.value = q
  if (typeof cand === 'string' && /^\d+$/.test(cand)) {
    pendingCandidateId = Number(cand)
    showDetail.value = true
  }
  void reloadAll()
})

onBeforeUnmount(() => clearTimeout(syncTimer))

// ---- 上一个 / 下一个候选人切换（基于当前搜索后的名册顺序） ----
const selectedIndex = computed(() => filtered.value.findIndex((c) => c.id === selectedId.value))
const canPrev = computed(() => selectedIndex.value > 0)
const canNext = computed(() => selectedIndex.value !== -1 && selectedIndex.value < filtered.value.length - 1)
const prevCandidate = computed(() => (canPrev.value ? filtered.value[selectedIndex.value - 1] : null))
const nextCandidate = computed(() => (canNext.value ? filtered.value[selectedIndex.value + 1] : null))

function goPrev() {
  if (prevCandidate.value) select(prevCandidate.value)
}

function goNext() {
  if (nextCandidate.value) select(nextCandidate.value)
}

// 键盘 ←/→ 切换候选人；忽略输入控件聚焦时。
function onGlobalKeydown(e: KeyboardEvent) {
  const el = e.target as HTMLElement | null
  if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable)) return
  if (e.key === 'ArrowLeft') return goPrev()
  if (e.key === 'ArrowRight') return goNext()
}

onMounted(() => window.addEventListener('keydown', onGlobalKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKeydown))

// ---- 名册键盘导航：roving tabindex，方向键在列表内移动 ----
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
  select(next)
  void requestAnimationFrame(() => {
    const btns = rosterEl.value?.querySelectorAll<HTMLElement>('[role="option"]')
    const i = list.findIndex((c) => c.id === next.id)
    btns?.[i]?.focus()
  })
}

// ---- 行内出价编辑：草稿按候选人 id 存数值（NumberField 绑定），出价列表每次重拉后整体重置 ----
const drafts = ref<Record<number, number | null>>({})
const savingId = ref<number | null>(null)

watch(bidsByCandidate, (map) => {
  const next: Record<number, number | null> = {}
  for (const [candidateId, bid] of map) {
    next[candidateId] = bid.amount
  }
  drafts.value = next
}, { immediate: true })

async function saveBid(c: Candidate) {
  if (savingId.value !== null) return
  const amount = drafts.value[c.id]
  if (amount == null || !Number.isInteger(amount) || amount <= 0) {
    toast.error('出价必须为正整数')
    return
  }
  savingId.value = c.id
  try {
    await leftoverApi.setBid(c.id, amount)
    toast.success(`「${c.name}」出价已保存`)
    // 出价影响本部门 spent/remaining，总览一并重拉。
    await Promise.all([loadBids(), loadOverview()])
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    savingId.value = null
  }
}


/** 出价编辑区（详情卡内）：可出价时行内编辑，否则只读展示。 */
const bidEditable = computed(() => {
  const c = selectedCandidate.value
  if (!c) return false
  return canBid.value && !resultsByCandidate.value.has(c.id)
})

// ---- 结算（candidates.manage，仅捡漏阶段）：确认后取最高出价成交 ----
const resolveTarget = ref<Candidate | null>(null)
const resolving = ref(false)

async function confirmResolve() {
  const target = resolveTarget.value
  if (!target || resolving.value) return
  resolving.value = true
  try {
    const r = await leftoverApi.resolve(target.id)
    const deptName = deptNameById.value.get(r.department_id) ?? `部门#${r.department_id}`
    toast.success(`「${target.name}」已结算：${deptName} · ${r.amount}`)
    resolveTarget.value = null
    await reloadAll()
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    resolving.value = false
  }
}

/** 详情卡结算结果 Badge：仅展示已正式落库的成交结果；未成交不显示领先价。 */
const selectedResultBadge = computed(() => {
  const c = selectedCandidate.value
  if (!c) return null
  const r = resultsByCandidate.value.get(c.id)
  if (!r) return null
  const deptName = deptNameById.value.get(r.department_id) ?? `部门#${r.department_id}`
  return { label: `成交 ${deptName} · ${r.amount}`, variant: 'default' as const }
})

/** 名册项右侧 Badge（可多枚）：出价 / 成交结果。领先价不展示（对他部门保密）。 */
function rosterBadges(c: Candidate): { label: string; variant: 'default' | 'secondary' | 'outline' }[] {
  const badges: { label: string; variant: 'default' | 'secondary' | 'outline' }[] = []
  const mine = bidsByCandidate.value.get(c.id)
  if (mine) badges.push({ label: `出价 ${mine.amount}`, variant: 'outline' })
  const r = resultsByCandidate.value.get(c.id)
  if (r) {
    const deptName = deptNameById.value.get(r.department_id) ?? `部门#${r.department_id}`
    badges.push({ label: `成交 ${deptName} · ${r.amount}`, variant: 'default' })
  }
  return badges
}

// ---- 移动端：列表 / 详情两段式切换（桌面端恒为分栏） ----
const showDetail = ref(false)
const asideClass = computed(() => (showDetail.value ? 'hidden lg:flex' : 'flex'))
const detailClass = computed(() => (showDetail.value ? 'flex' : 'hidden lg:flex'))
</script>

<template>
  <!-- 整体居中并限制最大宽度（max-w-content 语义档位），左右分栏 -->
  <div class="mx-auto flex h-full w-full max-w-content overflow-hidden">
    <!-- 左侧：候选人名册（无边框、底色与右侧一致） -->
    <aside
      :class="cn(asideClass, 'h-full min-w-0 shrink-0 flex-col lg:w-80')"
      aria-label="捡漏候选人名册"
    >
      <!-- 紧凑工具条：标题 + 结果数 + 刷新 -->
      <div class="flex items-center gap-2 p-3">
        <h1 class="flex min-w-0 items-center gap-2 truncate text-base font-semibold tracking-tight">
          <Gavel class="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          捡漏竞拍
        </h1>
        <Badge :variant="isLeftoverPhase ? 'default' : 'secondary'">{{ phaseLabel }}</Badge>
        <span class="ml-auto flex shrink-0 items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            class="h-8 w-8"
            :class="showSettled ? 'text-primary' : ''"
            :aria-pressed="showSettled"
            :aria-label="showSettled ? '隐藏已录取候选人' : '显示已录取候选人'"
            :title="showSettled ? '隐藏已录取候选人' : '显示已录取候选人'"
            @click="showSettled = !showSettled"
          >
            <Eye v-if="showSettled" aria-hidden="true" />
            <EyeOff v-else aria-hidden="true" />
          </Button>
          <Button variant="ghost" size="icon" class="h-8 w-8" aria-label="刷新捡漏信息" :disabled="loading" @click="reloadAll">
            <RefreshCw :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
          </Button>
        </span>
      </div>

      <div class="px-3 pb-2">
        <SearchInput v-model="keyword" full placeholder="搜索姓名…" />
      </div>

      <!-- 名册滚动区 -->
      <ScrollArea class="min-h-0 flex-1">
        <div ref="rosterEl" role="listbox" aria-label="候选人列表" class="space-y-1 p-2" @keydown="onRosterKeydown">
          <!-- 加载骨架屏 -->
          <div v-if="poolLoading && candidates.length === 0" class="space-y-2 p-2" aria-busy="true">
            <Skeleton v-for="i in 5" :key="i" class="h-14 w-full rounded-md" />
          </div>

          <!-- 拉取失败 -->
          <div v-else-if="poolError" role="alert" class="p-4 text-sm text-destructive">
            候选人加载失败：{{ poolError }}
            <Button variant="link" class="h-auto p-0" @click="reloadAll">重试</Button>
          </div>

          <!-- 空态 -->
          <EmptyState v-else-if="filtered.length === 0" bare :icon="hasFilter ? SearchX : UsersRound" class="py-10">
            <template v-if="hasFilter">没有匹配的候选人</template>
            <template v-else-if="!showSettled && candidates.length">待竞拍候选人已全部录取</template>
            <template v-else>暂无已完成候选人</template>
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
                <span v-if="rosterBadges(c).length" class="flex shrink-0 items-center gap-1">
                  <Badge v-for="b in rosterBadges(c)" :key="b.label" :variant="b.variant">
                    {{ b.label }}
                  </Badge>
                </span>
              </span>
              <span class="mt-0.5 flex items-center justify-between gap-2 text-xs text-muted-foreground">
                <span class="min-w-0 truncate">{{ c.profile || '无简介' }}</span>
              </span>
            </span>
          </button>
        </div>
      </ScrollArea>
    </aside>

    <!-- 右侧：预算 + 选中候选人的出价与结算详情 -->
    <section
      :class="cn(detailClass, 'h-full min-w-0 flex-1 flex-col')"
      aria-label="捡漏详情"
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
          <!-- 预算卡：本部门 预算/已出/剩余（无部门用户只读，标题自明） -->
          <div v-if="overviewError && !overview" role="alert" class="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
            预算信息加载失败：{{ overviewError }}
            <Button variant="link" class="h-auto p-0" @click="loadOverview">重试</Button>
          </div>
          <template v-else-if="overview">
            <Card v-if="overview.my || !canBrowseAll">
              <CardHeader class="flex-row items-center justify-between space-y-0">
                <CardTitle class="text-base">{{ overview.my ? '本部门预算' : '未分配部门' }}</CardTitle>
                <Badge :variant="isLeftoverPhase ? 'default' : 'secondary'">{{ phaseLabel }}</Badge>
              </CardHeader>
              <CardContent v-if="overview.my" class="pt-0">
                <div class="flex flex-wrap items-center gap-x-8 gap-y-2">
                  <div class="flex items-baseline gap-2">
                    <span class="text-sm text-muted-foreground">预算</span>
                    <span class="font-mono text-lg font-semibold">{{ overview.my.budget }}</span>
                  </div>
                  <div class="flex items-baseline gap-2">
                    <span class="text-sm text-muted-foreground">已出</span>
                    <span class="font-mono text-lg font-semibold">{{ overview.my.spent }}</span>
                  </div>
                  <div class="flex items-baseline gap-2">
                    <span class="text-sm text-muted-foreground">剩余</span>
                    <span class="font-mono text-lg font-semibold">{{ overview.my.remaining }}</span>
                  </div>
                </div>
              </CardContent>
            </Card>

            <!-- 管理端（browse_all）：各部门预算占用一览 -->
            <Card v-if="canBrowseAll">
              <CardHeader class="flex-row items-center justify-between space-y-0">
                <CardTitle class="text-base">各部门预算</CardTitle>
              </CardHeader>
              <CardContent class="pt-0">
                <div class="divide-y divide-border">
                  <div
                    v-for="d in overview.departments"
                    :key="d.id"
                    class="flex flex-wrap items-center justify-between gap-x-8 gap-y-2 py-2"
                  >
                    <span class="text-sm font-medium">{{ d.name }}</span>
                    <div class="flex flex-wrap items-center gap-x-8 gap-y-2">
                      <div class="flex items-baseline gap-2">
                        <span class="text-sm text-muted-foreground">预算</span>
                        <span class="font-mono text-sm font-semibold">{{ d.budget }}</span>
                      </div>
                      <div class="flex items-baseline gap-2">
                        <span class="text-sm text-muted-foreground">已出</span>
                        <span class="font-mono text-sm font-semibold">{{ d.spent ?? '-' }}</span>
                      </div>
                      <div class="flex items-baseline gap-2">
                        <span class="text-sm text-muted-foreground">剩余</span>
                        <span class="font-mono text-sm font-semibold">{{ d.remaining ?? '-' }}</span>
                      </div>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </template>

          <!-- 选中候选人的出价与结算详情 -->
          <EmptyState v-if="!selectedCandidate" bare :icon="Gavel" class="py-20">
            从左侧选择一位候选人查看出价与结算
          </EmptyState>

          <template v-else>
            <Card>
              <CardHeader class="flex-row items-start gap-4 space-y-0">
                <Avatar :class="avatarVariants({ size: 'lg' })" aria-hidden="true">
                  <AvatarFallback>{{ initialsOf(selectedCandidate.name) }}</AvatarFallback>
                </Avatar>
                <div class="min-w-0 flex-1 space-y-1.5">
                  <div class="flex flex-wrap items-center gap-2">
                    <CardTitle class="truncate text-xl">{{ selectedCandidate.name }}</CardTitle>
                    <Badge v-if="selectedResultBadge" :variant="selectedResultBadge.variant">
                      {{ selectedResultBadge.label }}
                    </Badge>
                  </div>
                  <p class="text-sm text-muted-foreground">
                    {{ selectedCandidate.profile || '暂无个人简介' }}
                  </p>
                </div>
                <Button
                  v-if="canManage && isLeftoverPhase && !resultsByCandidate.has(selectedCandidate.id)"
                  size="sm"
                  variant="outline"
                  class="shrink-0"
                  @click="resolveTarget = selectedCandidate"
                >
                  <Gavel aria-hidden="true" />
                  结算
                </Button>
              </CardHeader>
              <CardContent class="pt-0">
                <div class="space-y-2 pt-4 text-sm">
                  <!-- 出价：可出价时行内编辑（数字输入，Enter / 按钮保存），否则只读展示 -->
                  <div class="flex items-center justify-between gap-3">
                    <span class="text-muted-foreground">出价</span>
                    <template v-if="bidEditable">
                      <div class="flex items-center gap-2">
                        <NumberField
                          :model-value="drafts[selectedCandidate.id] ?? null"
                          :step="bidStep"
                          :min="bidStep"
                          :format-options="{ useGrouping: false }"
                          :disabled="savingId === selectedCandidate.id"
                          @update:model-value="drafts[selectedCandidate.id] = $event"
                        >
                          <NumberFieldContent>
                            <NumberFieldDecrement />
                            <NumberFieldInput
                              class="h-8"
                              :aria-label="`「${selectedCandidate.name}」的出价（上下键按 ${bidStep} 调整）`"
                              @keydown.enter="saveBid(selectedCandidate)"
                            />
                            <NumberFieldIncrement />
                          </NumberFieldContent>
                        </NumberField>
                        <Button
                          size="icon"
                          variant="outline"
                          class="h-8 w-8"
                          :disabled="savingId === selectedCandidate.id"
                          :aria-label="`保存「${selectedCandidate.name}」的出价`"
                          @click="saveBid(selectedCandidate)"
                        >
                          <Check aria-hidden="true" />
                        </Button>
                      </div>
                    </template>
                    <span v-else-if="bidsByCandidate.get(selectedCandidate.id)" class="font-mono">
                      {{ bidsByCandidate.get(selectedCandidate.id)!.amount }}
                    </span>
                    <span v-else class="text-muted-foreground">-</span>
                  </div>
                </div>

                <!-- 各部门出价（仅 browse_all 管理端可见）：按金额降序的「部门 · 金额」Badge -->
                <template v-if="canBrowseAll">
                  <div class="mt-4 space-y-2">
                    <span class="text-sm text-muted-foreground">各部门出价</span>
                    <div class="flex flex-wrap items-center gap-2">
                      <template v-if="(allBidsByCandidate.get(selectedCandidate.id) ?? []).length">
                        <Badge
                          v-for="(b, i) in allBidsByCandidate.get(selectedCandidate.id)"
                          :key="b.id"
                          :variant="i === 0 ? 'default' : 'secondary'"
                        >
                          {{ deptNameById.get(b.department_id) ?? `部门#${b.department_id}` }} · {{ b.amount }}
                        </Badge>
                      </template>
                      <span v-else class="text-sm text-muted-foreground">-</span>
                    </div>
                  </div>
                </template>
              </CardContent>
            </Card>

            <!-- 记录末尾：上一个 / 下一个候选人（←/→ 键盘可达） -->
            <nav v-if="filtered.length > 0" class="flex items-center justify-between gap-3" aria-label="候选人切换">
              <Button
                variant="outline"
                class="min-w-0 max-w-[45%] gap-1.5"
                :disabled="!canPrev"
                @click="goPrev"
              >
                <ChevronLeft class="shrink-0" aria-hidden="true" />
                <span class="min-w-0 truncate">上一个</span>
              </Button>
              <Button
                variant="outline"
                class="min-w-0 max-w-[45%] gap-1.5"
                :disabled="!canNext"
                @click="goNext"
              >
                <span class="min-w-0 truncate">下一个</span>
                <ChevronRight class="shrink-0" aria-hidden="true" />
              </Button>
            </nav>
          </template>
        </div>
      </ScrollArea>
    </section>

    <!-- 结算确认：出价最高部门赢得该候选人 -->
    <ConfirmDialog
      :open="!!resolveTarget"
      :title="resolveTarget ? `结算「${resolveTarget.name}」` : ''"
      confirm-text="结算"
      :loading="resolving"
      @update:open="resolveTarget = $event ? resolveTarget : null"
      @confirm="confirmResolve"
    />
  </div>
</template>
