<!--
  SettingsSystemStatusView —— 系统状态设置：在「面试阶段 / 录取阶段 / 捡漏阶段 / 结算阶段」之间切换。
  阶段呈 Stepper 进度条：已越过的档带勾选、当前档高亮、后续档待激活，点击任意档即切换。
  录取/捡漏阶段时下方展示录取情况预览：全体候选人 × 各部门决定的矩阵与汇总结论
  （未表态部门按弃权计入 → 「录取到该部门」）。跨部门数据需 candidates.browse_all。
  **捡漏/结算阶段**另加两列：竞拍情况（各部门当前出价）与预览录取结果（当前最高出价部门 +
  是否已结算，取自 GET /api/leftover/projections）；无出价的候选人回退到决定矩阵的结论。
-->
<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Check, UsersRound } from 'lucide-vue-next'
import { createColumnHelper } from '@tanstack/vue-table'

import { admissionApi, candidateApi, departmentApi, leftoverApi, systemStatusApi } from '@/api/http'
import { useBoardChannel } from '@/composables/useBoardChannel'
import { useSystemStatus } from '@/composables/useSystemStatus'
import { buildAdmissionPreview, type AdmissionPreviewRow } from '@/domain/admission'
import { ADMISSION_PRESENTATION, PHASE_PRESENTATION, admissionOutcomePresentation } from '@/presenters/status'
import type {
  SystemPhase,
  Bid,
  Candidate,
  CandidateAdmission,
  Department,
  LeftoverFinalResult,
} from '@/models'
import { PERMISSIONS, SYSTEM_PHASES } from '@/models'
import { useAuth } from '@/composables/useAuth'
import { toastError } from '@/lib/toast'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { NumberField, NumberFieldContent, NumberFieldDecrement, NumberFieldIncrement, NumberFieldInput } from '@/components/ui/number-field'
import DataTable from '@/components/ui/table/data-table.vue'
import type { DataTableFeatures } from '@/components/ui/table/features'
import {
  Stepper,
  StepperIndicator,
  StepperItem,
  StepperSeparator,
  StepperTitle,
  StepperTrigger,
} from '@/components/ui/stepper'
import EmptyState from '@/components/app/EmptyState.vue'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import ListSkeleton from '@/components/app/ListSkeleton.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'

const { status, loading, load, setPhase } = useSystemStatus()
const switching = ref(false)

/** 阶段 → Stepper 档位序号（1 起，与状态机推进方向一致）。 */
const phaseSteps = SYSTEM_PHASES.map((phase, i) => ({ phase, step: i + 1 }))

/** 当前档位（受控于 :model-value，不直接响应用户点击）。 */
const activeStep = ref(1)

/** 进入结算阶段的二次确认（该切换会按出价结算全部竞拍）。 */
const settlementConfirmOpen = ref(false)

function stepToPhase(step: number | undefined): SystemPhase | null {
  return phaseSteps.find((it) => it.step === step)?.phase ?? null
}

function syncStep() {
  const phase = status.value?.phase
  if (!phase) return
  const item = phaseSteps.find((it) => it.phase === phase)
  if (item) activeStep.value = item.step
}

// 服务端阶段（加载/切换后刷新）是唯一权威，变化即回写当前档位。
watch(() => status.value?.phase, syncStep, { immediate: true })

function onStepChange(step: number | undefined) {
  const phase = stepToPhase(step)
  if (!phase) return
  // 进入结算阶段会按出价结算全部竞拍（封盘，不可回到未结算状态），故先二次确认。
  if (phase === 'settlement' && status.value?.phase !== 'settlement') {
    settlementConfirmOpen.value = true
    return
  }
  void switchTo(phase)
}

async function confirmSettlement() {
  settlementConfirmOpen.value = false
  await switchTo('settlement')
}

async function switchTo(phase: SystemPhase) {
  if (switching.value || status.value?.phase === phase) return
  switching.value = true
  try {
    await setPhase(phase)
    toast.success(`系统已切换为「${PHASE_PRESENTATION[phase].label}」`)
  } catch (e) {
    toastError(e)
  } finally {
    switching.value = false
  }
}

// ---- 录取情况预览（录取/捡漏阶段展示；预览为全局矩阵，需 candidates.browse_all） ----
const { hasPermission } = useAuth()
const canBrowseAll = computed(() => hasPermission(PERMISSIONS.CANDIDATES_BROWSE_ALL))


// ---- 出价步长（管理面板设置，服务端校验 ≥1） ----
const bidStepDraft = ref<number | null>(status.value?.bid_step ?? 10)
const bidStepSaving = ref(false)

watch(() => status.value?.bid_step, (v) => {
  if (v != null) bidStepDraft.value = v
}, { immediate: true })

async function saveBidStep() {
  const step = bidStepDraft.value
  if (step == null || !Number.isInteger(step) || step < 1) {
    toast.error('出价步长须为正整数')
    return
  }
  bidStepSaving.value = true
  try {
    await systemStatusApi.patch({ bid_step: step })
    toast.success(`出价步长已设为 ${step}`)
    await load()
  } catch (e) {
    toastError(e)
  } finally {
    bidStepSaving.value = false
  }
}
const previewCandidates = ref<Candidate[]>([])
const previewDepartments = ref<Department[]>([])
const previewAdmissions = ref<CandidateAdmission[]>([])
/** 竞拍数据（捡漏/结算阶段展示）：各部门出价 + 结算预览。 */
const previewBids = ref<Bid[]>([])
const previewFinals = ref<LeftoverFinalResult[]>([])
const previewLoading = ref(false)
const previewError = ref('')

const showPreview = computed(
  () =>
    canBrowseAll.value &&
    (status.value?.phase === 'admission' ||
      status.value?.phase === 'leftover' ||
      status.value?.phase === 'settlement'),
)

const previewRows = computed<AdmissionPreviewRow[]>(() =>
  buildAdmissionPreview(previewCandidates.value, previewDepartments.value, previewAdmissions.value),
)

// ---- 竞拍阶段派生：出价索引 / 结算预览索引 / 部门名 ----
/** 捡漏/结算阶段：表格额外展示「竞拍情况」与「预览录取结果」。 */
const inAuctionPhase = computed(
  () => status.value?.phase === 'leftover' || status.value?.phase === 'settlement',
)

/** 候选人 → 各部门出价（金额降序，管理端可见全部门）。 */
const bidsByCandidate = computed(() => {
  const map = new Map<number, Bid[]>()
  for (const b of previewBids.value) {
    const list = map.get(b.candidate_id) ?? []
    list.push(b)
    map.set(b.candidate_id, list)
  }
  for (const list of map.values()) list.sort((x, y) => y.amount - x.amount)
  return map
})

/** 候选人 → 结算预览（当前最高出价部门；resolved 表示已正式落库）。 */
const finalsByCandidate = computed(
  () => new Map(previewFinals.value.map((f) => [f.candidate_id, f])),
)

/** 部门 id → 名称（出价与预览列展示用）。 */
function deptLabelOf(departmentId: number): string {
  return previewDepartments.value.find((d) => d.id === departmentId)?.name ?? `部门#${departmentId}`
}

// ---- DataTable 列定义：候选人 + 各部门决定（动态列）+ 汇总结论 ----
const previewColumnHelper = createColumnHelper<DataTableFeatures, AdmissionPreviewRow>()

const previewColumns = computed(() =>
  previewColumnHelper.columns([
    previewColumnHelper.accessor('candidateName', {
      header: '候选人',
      cell: ({ getValue }) => h('div', { class: 'font-medium' }, getValue()),
    }),
    ...(inAuctionPhase.value
      ? [
          previewColumnHelper.display({
            id: 'bids',
            header: '竞拍情况',
            cell: ({ row }) => {
              const list = bidsByCandidate.value.get(row.original.candidateId) ?? []
              if (!list.length) return h('span', { class: 'text-muted-foreground' }, '无出价')
              return h(
                'div',
                { class: 'flex flex-wrap items-center gap-1' },
                list.map((b, i) =>
                  h(
                    Badge,
                    { key: b.id, variant: i === 0 ? 'default' : 'secondary' },
                    () => `${deptLabelOf(b.department_id)} · ${b.amount}`,
                  ),
                ),
              )
            },
          }),
        ]
      : []),
    ...previewDepartments.value.map((d, i) =>
      previewColumnHelper.display({
        id: `dept-${d.id}`,
        header: d.name,
        cell: ({ row }) => {
          const p = ADMISSION_PRESENTATION[row.original.statuses[i]]
          return h(Badge, { variant: p.badge }, () => p.label)
        },
      }),
    ),
    previewColumnHelper.display({
      id: 'outcome',
      header: inAuctionPhase.value ? '预览录取结果' : '录取情况',
      cell: ({ row }) => {
        // 竞拍阶段：有出价 → 展示当前最高出价部门的预览结果（并标注是否已结算）；
        // 无出价 → 回退到决定矩阵推出的结论。
        const final = finalsByCandidate.value.get(row.original.candidateId)
        if (inAuctionPhase.value && final) {
          return h('div', { class: 'flex flex-wrap items-center gap-1' }, [
            h(
              Badge,
              { variant: 'default' },
              () => `录取到 ${deptLabelOf(final.department_id)} · ${final.amount}`,
            ),
            ...(final.resolved ? [h(Badge, { variant: 'outline' }, () => '已结算')] : []),
          ])
        }
        const p = admissionOutcomePresentation(row.original.outcome)
        return h(Badge, { variant: p.badge }, () => p.label)
      },
    }),
  ]),
)

async function loadPreview() {
  if (!showPreview.value || previewLoading.value) return
  previewLoading.value = true
  previewError.value = ''
  try {
    const [cand, dept, adm, bids, finals] = await Promise.all([
      candidateApi.listAll(),
      departmentApi.list(),
      admissionApi.list(),
      leftoverApi.bids(),
      leftoverApi.projections(),
    ])
    previewCandidates.value = cand.items
    previewDepartments.value = dept.items
    previewAdmissions.value = adm.items
    previewBids.value = bids.items
    previewFinals.value = finals.items
  } catch (e) {
    previewError.value = (e as Error).message
  } finally {
    previewLoading.value = false
  }
}

// 进入录取/捡漏阶段即拉取预览；录取决定无看板事件，仅候选人增删触发重拉（其余靠手动刷新）。
// 竞拍数据有看板事件（leftover_bid / leftover_resolved），出价与结算即时反映。
watch(showPreview, (show) => {
  if (show) void loadPreview()
})

const PREVIEW_RELOAD_EVENTS = [
  'candidate_created',
  'candidate_updated',
  'candidate_deleted',
  'leftover_bid',
  'leftover_resolved',
]
let previewTimer: ReturnType<typeof setTimeout> | undefined
useBoardChannel().subscribe((ev) => {
  if (showPreview.value && PREVIEW_RELOAD_EVENTS.includes(ev.type)) {
    clearTimeout(previewTimer)
    previewTimer = setTimeout(() => void loadPreview(), 300)
  }
})

onMounted(() => void load())
</script>

<template>
  <PageShell title="系统状态">
    <template #actions>
      <RefreshButton label="刷新系统状态" :loading="loading" @click="load" />
    </template>

    <ListSkeleton v-if="loading && !status" :rows="2" item-class="h-28 w-full rounded-xl" class="space-y-4" />

    <template v-else>
      <Card class="p-5">
        <Stepper
          :model-value="activeStep"
          :linear="false"
          class="flex w-full items-start gap-2"
          @update:model-value="onStepChange"
        >
          <StepperItem
            v-for="(item, i) in phaseSteps"
            :key="item.phase"
            v-slot="{ state }"
            :step="item.step"
            :disabled="switching"
            class="relative flex w-full flex-col items-center justify-center"
          >
            <StepperSeparator
              v-if="i < phaseSteps.length - 1"
              class="absolute left-[calc(50%+20px)] right-[calc(-50%+10px)] top-5 block h-0.5 shrink-0 rounded-full bg-muted group-data-[state=completed]:bg-primary"
            />
            <StepperTrigger>
              <StepperIndicator>
                <Check v-if="state === 'completed'" class="h-4 w-4" aria-hidden="true" />
                <component
                  :is="PHASE_PRESENTATION[item.phase].icon"
                  v-else
                  class="h-4 w-4 shrink-0"
                  aria-hidden="true"
                />
              </StepperIndicator>
            </StepperTrigger>
            <StepperTitle
              class="mt-2"
              :class="state === 'active' ? 'text-primary' : ''"
            >
              {{ PHASE_PRESENTATION[item.phase].label }}
            </StepperTitle>
          </StepperItem>
        </Stepper>
      </Card>

      <!-- 出价步长（users.manage）：上下键调整报价的步进 -->
      <Card class="p-5">
        <div class="flex flex-wrap items-center justify-between gap-x-8 gap-y-3">
          <h2 class="text-base font-semibold">出价步长</h2>
          <div class="flex items-center gap-2">
            <NumberField
              :model-value="bidStepDraft"
              :step="1"
              :min="1"
              class="w-28"
              @update:model-value="bidStepDraft = $event"
            >
              <NumberFieldContent>
                <NumberFieldDecrement />
                <NumberFieldInput aria-label="出价步长" />
                <NumberFieldIncrement />
              </NumberFieldContent>
            </NumberField>
            <Button size="sm" :disabled="bidStepSaving || Number(bidStepDraft) === (status?.bid_step ?? 0)" @click="saveBidStep">
              保存
            </Button>
          </div>
        </div>
      </Card>

      <!-- 录取情况预览：全体候选人 × 各部门决定 + 汇总结论 -->
      <div v-if="showPreview">
        <div class="mb-4 flex items-center justify-between gap-2">
          <h2 class="text-base font-semibold">录取情况预览</h2>
          <RefreshButton label="刷新录取情况" :loading="previewLoading" @click="loadPreview" />
        </div>

        <ListSkeleton v-if="previewLoading && previewRows.length === 0" :rows="3" />

        <EmptyState v-else-if="previewError" :icon="UsersRound">
          录取情况加载失败：{{ previewError }}
        </EmptyState>

        <EmptyState v-else-if="previewRows.length === 0" :icon="UsersRound">
          暂无候选人
        </EmptyState>

        <DataTable v-else :columns="previewColumns" :data="previewRows" />
      </div>
    </template>

    <!-- 进入结算阶段：按出价结算全部竞拍（封盘） -->
    <ConfirmDialog
      :open="settlementConfirmOpen"
      title="进入结算阶段并按出价结算全部竞拍"
      confirm-text="结算并切换"
      :loading="switching"
      @update:open="settlementConfirmOpen = $event"
      @confirm="confirmSettlement"
    />
  </PageShell>
</template>
