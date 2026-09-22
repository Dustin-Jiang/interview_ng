<!--
  LeftoverView —— 捡漏竞拍（布局与 CandidateRecordsView 一致：左名册 + 右详情，移动端两段式）。
  左侧：COMPLETED 候选人名册（搜索 + 刷新 + 键盘可达的 listbox）；
  右侧：预算卡（本部门 / browse_all 各部门）+ 选中候选人的出价与结算详情。
  出价需 admissions.record 且已分配部门且处于捡漏阶段（行内数字输入 + ←/→ 步进 + Enter/按钮保存）；
  键位：**↑/↓ 切换候选人**、**←/→ 调整报价**（焦点在出价输入框内同样生效）、Enter 保存。
  结算不由本页触发：进入「结算阶段」时后端按出价自动结算全部竞拍，结算结果以带文字 Badge 呈现。
  结算阶段竞拍数据只读。无部门用户只读。

  组装层：布局与名册交给 MasterDetailSplit / RosterToolbar / RosterList / RosterPager，
  数据与交互交给 useLeftover / useRosterSelection / useRosterHotkeys。
-->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Check, Eye, EyeOff, Gavel, Minus, Plus, SearchX, UsersRound } from 'lucide-vue-next'

import { useLeftover } from '@/composables/useLeftover'
import { useRosterHotkeys } from '@/composables/useRosterHotkeys'
import { useRosterRouteSync } from '@/composables/useRosterRouteSync'
import { useRosterSelection } from '@/composables/useRosterSelection'
import { isActivatableElement } from '@/lib/dom'
import type { Candidate } from '@/models'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import CandidateDetailHeader from '@/components/app/CandidateDetailHeader.vue'
import EmptyState from '@/components/app/EmptyState.vue'
import ErrorAlert from '@/components/app/ErrorAlert.vue'
import MasterDetailSplit from '@/components/app/MasterDetailSplit.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'
import RosterList from '@/components/app/RosterList.vue'
import RosterPager from '@/components/app/RosterPager.vue'
import RosterToolbar from '@/components/app/RosterToolbar.vue'
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
  canBrowseAll,
  canBid,
  bidStep,
  drafts,
  savingId,
  setDraft,
  stepDraft,
  saveBid,
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
    return c.student_no.includes(kw) || c.name.toLowerCase().includes(kw)
  })
})

const emptyText = computed(() => {
  if (hasFilter.value) return '没有匹配的候选人'
  if (!showSettled.value && candidates.value.length) return '待竞拍候选人已全部录取'
  return '暂无已完成候选人'
})

const { selectedId, selected, select, canPrev, canNext, goPrev, goNext, showDetail, closeDetail, presetSelection } =
  useRosterSelection<Candidate>({
    items: () => filtered.value,
    lookup: (id) => candidates.value.find((c) => c.id === id) ?? null,
    ready: () => !poolLoading.value,
  })

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

// ---- 键盘：↑/↓ 切换候选人、←/→ 调整报价、Enter 保存 ----
// 名册与出价输入框都不再自带方向键，键位统一在本页处理；两个名册页一致：↑/↓ 切人、←/→ 调价。
// 焦点在**出价输入框**内时同样生效（该框只放数字，不需要左右移动光标）；其它输入控件（搜索框等）完全让位。
const BID_INPUT_ATTR = 'bidInput'

/** 出价输入框：唯一在焦点内仍响应方向键的控件（标记 `data-bid-input`）。 */
function isBidInput(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  return !!el && el.dataset?.[BID_INPUT_ATTR] !== undefined
}

useRosterHotkeys({
  goPrev,
  goNext,
  isHotkeyInput: isBidInput,
  onOtherKey: (e, inBidInput) => {
    const c = selected.value
    const editable = !!c && bidEditable.value
    if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
      if (!editable || !c) return false
      stepDraft(c, e.key === 'ArrowLeft' ? -1 : 1)
      return true
    }
    if (e.key !== 'Enter' || !editable || !c) return false
    if (!inBidInput && isActivatableElement(e.target)) return false // 真按钮/链接：Enter 归它们
    void saveBid(c)
    return true
  },
})

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
      <RosterToolbar title="捡漏竞拍" :icon="Gavel">
        <template #meta>
          <Badge :variant="isLeftoverPhase ? 'default' : 'secondary'">{{ phaseLabel }}</Badge>
        </template>
        <template #actions>
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
        </template>
      </RosterToolbar>

      <div class="px-3 pb-2">
        <SearchInput v-model="keyword" full placeholder="搜索学号 / 姓名…" />
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
        @retry="reloadAll"
      >
        <template #badges="{ item }">
          <Badge v-for="b in rosterBadges(item)" :key="b.label" :variant="b.variant" class="max-w-full">
            <span class="truncate">{{ b.label }}</span>
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
          <div class="space-y-2 pt-4 text-sm">
            <!-- 出价：可出价时行内编辑（←/→ 步进、Enter / 按钮保存），否则只读展示。
                 手机上「标签 / 步进组」可折行，避免 w-20 输入框 + 三个按钮撑破卡片 -->
            <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
              <span class="shrink-0 text-muted-foreground">出价</span>
              <template v-if="bidEditable">
                <div class="flex items-center gap-1">
                  <Button
                    size="icon"
                    variant="outline"
                    class="h-8 w-8"
                    :disabled="savingId === selected.id"
                    aria-label="减少出价（←）"
                    @click="stepDraft(selected, -1)"
                  >
                    <Minus aria-hidden="true" />
                  </Button>
                  <Input
                    :model-value="drafts[selected.id] ?? ''"
                    data-bid-input
                    inputmode="numeric"
                    autocomplete="off"
                    class="h-8 w-20 text-center tabular-nums"
                    :disabled="savingId === selected.id"
                    :aria-label="`「${selected.name}」的出价（←/→ 按 ${bidStep} 调整，Enter 保存）`"
                    @update:model-value="setDraft(selected, String($event))"
                  />
                  <Button
                    size="icon"
                    variant="outline"
                    class="h-8 w-8"
                    :disabled="savingId === selected.id"
                    aria-label="增加出价（→）"
                    @click="stepDraft(selected, 1)"
                  >
                    <Plus aria-hidden="true" />
                  </Button>
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
                    class="max-w-full"
                  >
                    <span class="truncate">{{ deptName(b.department_id) }} · {{ b.amount }}</span>
                  </Badge>
                </template>
                <span v-else class="text-sm text-muted-foreground">-</span>
              </div>
            </div>
          </template>
        </CandidateDetailHeader>

        <!-- 记录末尾：上一个 / 下一个候选人（↑/↓ 键盘可达） -->
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
