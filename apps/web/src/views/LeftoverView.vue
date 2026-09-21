<!--
  LeftoverView —— 捡漏竞拍（布局与 CandidateRecordsView 一致：左名册 + 右详情，移动端两段式）。
  左侧：COMPLETED 候选人名册（搜索 + 刷新 + 键盘可达的 listbox）；
  右侧：预算卡（本部门 / browse_all 各部门）+ 选中候选人的出价与结算详情。
  出价需 admissions.record 且已分配部门且处于捡漏阶段（行内数字输入 + Enter/按钮保存）；
  结算按钮仅对持 candidates.manage 的用户且处于捡漏阶段出现（ConfirmDialog 确认，
  赢家为最高出价部门）；结算结果以带文字 Badge 呈现。结算阶段竞拍数据只读。无部门用户只读。

  组装层：布局与名册交给 MasterDetailSplit / RosterList / RosterPager，
  数据与交互交给 useLeftover / useRosterSelection / useConfirmAction。
-->
<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Check, Eye, EyeOff, Gavel, SearchX, UsersRound } from 'lucide-vue-next'

import { useConfirmAction } from '@/composables/useConfirmAction'
import { useLeftover } from '@/composables/useLeftover'
import { useRosterRouteSync } from '@/composables/useRosterRouteSync'
import { useRosterSelection } from '@/composables/useRosterSelection'
import type { Candidate } from '@/models'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  NumberField,
  NumberFieldContent,
  NumberFieldDecrement,
  NumberFieldIncrement,
  NumberFieldInput,
} from '@/components/ui/number-field'
import CandidateDetailHeader from '@/components/app/CandidateDetailHeader.vue'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import EmptyState from '@/components/app/EmptyState.vue'
import ErrorAlert from '@/components/app/ErrorAlert.vue'
import MasterDetailSplit from '@/components/app/MasterDetailSplit.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'
import RosterList from '@/components/app/RosterList.vue'
import RosterPager from '@/components/app/RosterPager.vue'
import SearchInput from '@/components/app/SearchInput.vue'

const route = useRoute()

const {
  overview,
  overviewError,
  candidates,
  bidsByCandidate,
  allBidsByCandidate,
  resultsByCandidate,
  loading,
  poolLoading,
  error,
  deptName,
  reloadAll,
  phaseLabel,
  isLeftoverPhase,
  canManage,
  canBrowseAll,
  canBid,
  bidStep,
  drafts,
  savingId,
  saveBid,
  resolveCandidate,
} = useLeftover()

// ---- 名册：搜索 + 已录取筛选 + 选中（入口 /leftover/candidates/:candidateId，可深链分享） ----
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

const emptyText = computed(() => {
  if (hasFilter.value) return '没有匹配的候选人'
  if (!showSettled.value && candidates.value.length) return '待竞拍候选人已全部录取'
  return '暂无已完成候选人'
})

const { selectedId, selected, select, highlight, canPrev, canNext, goPrev, goNext, showDetail, closeDetail, presetSelection } =
  useRosterSelection<Candidate>({
    items: () => filtered.value,
    lookup: (id) => candidates.value.find((c) => c.id === id) ?? null,
    ready: () => !poolLoading.value,
  })

// ---- 结算：确认后取最高出价成交 ----
const {
  target: resolveTarget,
  loading: resolving,
  request: requestResolve,
  onOpenChange: onResolveOpenChange,
  confirm: confirmResolve,
} = useConfirmAction<Candidate>({ action: resolveCandidate })

// ---- 展示派生：名册徽章 / 详情徽章 / 出价可编辑 ----
/** 名册项右侧 Badge（可多枚）：出价 / 成交结果。领先价不展示（对他部门保密）。 */
function rosterBadges(c: Candidate): { label: string; variant: 'default' | 'secondary' | 'outline' }[] {
  const badges: { label: string; variant: 'default' | 'secondary' | 'outline' }[] = []
  const mine = bidsByCandidate.value.get(c.id)
  if (mine) badges.push({ label: `出价 ${mine.amount}`, variant: 'outline' })
  const r = resultsByCandidate.value.get(c.id)
  if (r) badges.push({ label: `成交 ${deptName(r.department_id)} · ${r.amount}`, variant: 'default' })
  return badges
}

/** 详情卡结算结果 Badge：仅展示已正式落库的成交结果；未成交不显示领先价。 */
const selectedResultBadge = computed(() => {
  const c = selected.value
  if (!c) return null
  const r = resultsByCandidate.value.get(c.id)
  if (!r) return null
  return { label: `成交 ${deptName(r.department_id)} · ${r.amount}`, variant: 'default' as const }
})

/** 出价可编辑：可出价且该候选人尚未成交（成交后竞拍数据只读）。 */
const bidEditable = computed(
  () => !!selected.value && canBid.value && !resultsByCandidate.value.has(selected.value.id),
)

// ---- 深链：挂载恢复搜索（query）与选中条目（路径参数）；变更写回 URL（replace） ----
useRosterRouteSync<Candidate>({
  listRoute: 'leftover',
  itemRoute: 'leftover-candidate',
  selection: { selectedId, select, presetSelection },
  lookup: (id) => candidates.value.find((c) => c.id === id) ?? null,
  query: searchQuery,
  onMount: () => {
    const q = route.query.q
    if (typeof q === 'string') keyword.value = q
    void reloadAll()
  },
})

/** 搜索态（query）——选中条目走路径参数。 */
function searchQuery(): Record<string, string> {
  const query: Record<string, string> = {}
  const kw = keyword.value.trim()
  if (kw) query.q = kw
  return query
}
</script>

<template>
  <MasterDetailSplit
    :show-detail="showDetail"
    aside-label="捡漏候选人名册"
    detail-label="捡漏详情"
    back-aria-label="返回候选人列表"
    @back="closeDetail"
  >
    <!-- 左侧：名册工具条 + 名册 -->
    <template #aside>
      <!-- 紧凑工具条：标题 + 阶段 + 已录取显隐 + 刷新 -->
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
          <RefreshButton
            variant="ghost"
            class="h-8 w-8"
            label="刷新捡漏信息"
            :loading="loading"
            @click="reloadAll"
          />
        </span>
      </div>

      <div class="px-3 pb-2">
        <SearchInput v-model="keyword" full placeholder="搜索姓名…" />
      </div>

      <RosterList
        :items="filtered"
        :selected-id="selectedId"
        :skeleton="poolLoading && candidates.length === 0"
        :error="error"
        :empty-text="emptyText"
        :empty-icon="hasFilter ? SearchX : UsersRound"
        list-label="候选人列表"
        @select="select"
        @highlight="highlight"
        @retry="reloadAll"
      >
        <template #badges="{ item }">
          <Badge v-for="b in rosterBadges(item)" :key="b.label" :variant="b.variant">
            {{ b.label }}
          </Badge>
        </template>
      </RosterList>
    </template>

    <!-- 右侧：预算 + 选中候选人的出价与结算详情 -->
    <template #detail>
      <!-- 预算卡：本部门 预算/已出/剩余（无部门用户只读，标题自明） -->
      <ErrorAlert
        v-if="overviewError && !overview"
        :message="`预算信息加载失败：${overviewError}`"
        retry-label="重试"
        @retry="reloadAll"
      />
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
      <EmptyState v-if="!selected" bare :icon="Gavel" class="py-20">
        从左侧选择一位候选人查看出价与结算
      </EmptyState>

      <template v-else>
        <CandidateDetailHeader :candidate="selected" :badge="selectedResultBadge">
          <template #actions>
            <Button
              v-if="canManage && isLeftoverPhase && !resultsByCandidate.has(selected.id)"
              size="sm"
              variant="outline"
              class="shrink-0"
              @click="requestResolve(selected)"
            >
              <Gavel aria-hidden="true" />
              结算
            </Button>
          </template>

          <div class="space-y-2 pt-4 text-sm">
            <!-- 出价：可出价时行内编辑（数字输入，Enter / 按钮保存），否则只读展示 -->
            <div class="flex items-center justify-between gap-3">
              <span class="text-muted-foreground">出价</span>
              <template v-if="bidEditable">
                <div class="flex items-center gap-2">
                  <NumberField
                    :model-value="drafts[selected.id] ?? null"
                    :step="bidStep"
                    :min="bidStep"
                    :format-options="{ useGrouping: false }"
                    :disabled="savingId === selected.id"
                    @update:model-value="drafts[selected.id] = $event"
                  >
                    <NumberFieldContent>
                      <NumberFieldDecrement />
                      <NumberFieldInput
                        class="h-8"
                        :aria-label="`「${selected.name}」的出价（上下键按 ${bidStep} 调整）`"
                        @keydown.enter="saveBid(selected)"
                      />
                      <NumberFieldIncrement />
                    </NumberFieldContent>
                  </NumberField>
                  <Button
                    size="icon"
                    variant="outline"
                    class="h-8 w-8"
                    :disabled="savingId === selected.id"
                    :aria-label="`保存「${selected.name}」的出价`"
                    @click="saveBid(selected)"
                  >
                    <Check aria-hidden="true" />
                  </Button>
                </div>
              </template>
              <span v-else-if="bidsByCandidate.get(selected.id)" class="font-mono">
                {{ bidsByCandidate.get(selected.id)!.amount }}
              </span>
              <span v-else class="text-muted-foreground">-</span>
            </div>
          </div>

          <!-- 各部门出价（仅 browse_all 管理端可见）：按金额降序的「部门 · 金额」Badge -->
          <template v-if="canBrowseAll">
            <div class="mt-4 space-y-2">
              <span class="text-sm text-muted-foreground">各部门出价</span>
              <div class="flex flex-wrap items-center gap-2">
                <template v-if="(allBidsByCandidate.get(selected.id) ?? []).length">
                  <Badge
                    v-for="(b, i) in allBidsByCandidate.get(selected.id)"
                    :key="b.id"
                    :variant="i === 0 ? 'default' : 'secondary'"
                  >
                    {{ deptName(b.department_id) }} · {{ b.amount }}
                  </Badge>
                </template>
                <span v-else class="text-sm text-muted-foreground">-</span>
              </div>
            </div>
          </template>
        </CandidateDetailHeader>

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

  <!-- 结算确认：出价最高部门赢得该候选人 -->
  <ConfirmDialog
    :open="!!resolveTarget"
    :title="resolveTarget ? `结算「${resolveTarget.name}」` : ''"
    confirm-text="结算"
    :loading="resolving"
    @update:open="onResolveOpenChange"
    @confirm="confirmResolve"
  />
</template>
