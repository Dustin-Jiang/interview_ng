<!--
  SettingsSystemStatusView —— 系统状态设置：在「面试阶段 / 录取阶段 / 捡漏阶段」之间切换。
  阶段呈 Stepper 进度条：已越过的档带勾选、当前档高亮、后续档待激活，点击任意档即切换。
  录取/捡漏阶段时下方展示录取情况预览：全体候选人 × 各部门决定的矩阵与汇总结论
  （唯一部门录取且其余全部放弃 → 「录取到该部门」）。跨部门数据需 candidates.browse_all。
-->
<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Check, RefreshCw, UserCheck, UserSearch, UsersRound } from 'lucide-vue-next'
import { createColumnHelper } from '@tanstack/vue-table'

import { admissionApi, candidateApi, departmentApi } from '@/api/http'
import { useBoardChannel } from '@/composables/useBoardChannel'
import { useSystemStatus } from '@/composables/useSystemStatus'
import { buildAdmissionPreview, type AdmissionPreviewRow } from '@/domain/admission'
import { ADMISSION_PRESENTATION, admissionOutcomePresentation } from '@/presenters/status'
import type { SystemPhase, Candidate, CandidateAdmission, Department } from '@/models'
import { PERMISSIONS, SYSTEM_PHASES } from '@/models'
import { useAuth } from '@/composables/useAuth'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
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
import PageShell from '@/components/app/PageShell.vue'

const { status, loading, load, setPhase } = useSystemStatus()
const switching = ref(false)

const phaseMeta: Record<SystemPhase, { label: string; icon: typeof UsersRound }> = {
  interview: { label: '面试阶段', icon: UsersRound },
  admission: { label: '录取阶段', icon: UserCheck },
  leftover: { label: '捡漏阶段', icon: UserSearch },
}

/** 阶段 → Stepper 档位序号（1 起，与状态机推进方向一致）。 */
const phaseSteps = SYSTEM_PHASES.map((phase, i) => ({ phase, step: i + 1 }))

/** 当前档位（受控于 :model-value，不直接响应用户点击）。 */
const activeStep = ref(1)

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
  if (phase) void switchTo(phase)
}

async function switchTo(phase: SystemPhase) {
  if (switching.value || status.value?.phase === phase) return
  switching.value = true
  try {
    await setPhase(phase)
    toast.success(`系统已切换为「${phaseMeta[phase].label}」`)
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    switching.value = false
  }
}

// ---- 录取情况预览（录取/捡漏阶段展示；预览为全局矩阵，需 candidates.browse_all） ----
const { hasPermission } = useAuth()
const canBrowseAll = computed(() => hasPermission(PERMISSIONS.CANDIDATES_BROWSE_ALL))

const previewCandidates = ref<Candidate[]>([])
const previewDepartments = ref<Department[]>([])
const previewAdmissions = ref<CandidateAdmission[]>([])
const previewLoading = ref(false)
const previewError = ref('')

const showPreview = computed(
  () =>
    canBrowseAll.value &&
    (status.value?.phase === 'admission' || status.value?.phase === 'leftover'),
)

const previewRows = computed<AdmissionPreviewRow[]>(() =>
  buildAdmissionPreview(previewCandidates.value, previewDepartments.value, previewAdmissions.value),
)

// ---- DataTable 列定义：候选人 + 各部门决定（动态列）+ 汇总结论 ----
const previewColumnHelper = createColumnHelper<DataTableFeatures, AdmissionPreviewRow>()

const previewColumns = computed(() =>
  previewColumnHelper.columns([
    previewColumnHelper.accessor('candidateName', {
      header: '候选人',
      cell: ({ getValue }) => h('div', { class: 'font-medium' }, getValue()),
    }),
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
      header: '录取情况',
      cell: ({ row }) => {
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
    const [cand, dept, adm] = await Promise.all([
      candidateApi.list({ limit: 200 }),
      departmentApi.list(),
      admissionApi.list(),
    ])
    previewCandidates.value = cand.items
    previewDepartments.value = dept.items
    previewAdmissions.value = adm.items
  } catch (e) {
    previewError.value = (e as Error).message
  } finally {
    previewLoading.value = false
  }
}

// 进入录取/捡漏阶段即拉取预览；录取决定无看板事件，仅候选人增删触发重拉（其余靠手动刷新）。
watch(showPreview, (show) => {
  if (show) void loadPreview()
})

const PREVIEW_RELOAD_EVENTS = ['candidate_created', 'candidate_updated', 'candidate_deleted']
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
      <Button variant="outline" size="icon" aria-label="刷新系统状态" @click="load">
        <RefreshCw :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
      </Button>
    </template>

    <div v-if="loading && !status" class="space-y-4" aria-busy="true">
      <Skeleton v-for="i in 2" :key="i" class="h-28 w-full rounded-xl" />
    </div>

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
                  :is="phaseMeta[item.phase].icon"
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
              {{ phaseMeta[item.phase].label }}
            </StepperTitle>
          </StepperItem>
        </Stepper>
      </Card>

      <!-- 录取情况预览：全体候选人 × 各部门决定 + 汇总结论 -->
      <Card v-if="showPreview" class="p-5">
        <div class="mb-4 flex items-center justify-between gap-2">
          <h2 class="text-base font-semibold">录取情况预览</h2>
          <Button
            variant="outline"
            size="icon"
            aria-label="刷新录取情况"
            @click="loadPreview"
          >
            <RefreshCw :class="previewLoading ? 'animate-spin' : ''" aria-hidden="true" />
          </Button>
        </div>

        <div v-if="previewLoading && previewRows.length === 0" class="space-y-2" aria-busy="true">
          <Skeleton v-for="i in 3" :key="i" class="h-12 w-full rounded-md" />
        </div>

        <EmptyState v-else-if="previewError" :icon="UsersRound">
          录取情况加载失败：{{ previewError }}
        </EmptyState>

        <EmptyState v-else-if="previewRows.length === 0" :icon="UsersRound">
          暂无候选人
        </EmptyState>

        <DataTable v-else :columns="previewColumns" :data="previewRows" />
      </Card>
    </template>
  </PageShell>
</template>
