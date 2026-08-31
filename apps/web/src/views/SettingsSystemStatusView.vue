<!--
  SettingsSystemStatusView —— 系统状态设置：在「面试阶段 / 录取阶段 / 捡漏阶段」之间切换。
  阶段呈 Stepper 进度条：已越过的档带勾选、当前档高亮、后续档待激活，点击任意档即切换。
-->
<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { toast } from 'vue-sonner'
import { Check, RefreshCw, UserCheck, UserSearch, UsersRound } from 'lucide-vue-next'

import { useSystemStatus } from '@/composables/useSystemStatus'
import type { SystemPhase } from '@/models'
import { SYSTEM_PHASES } from '@/models'

import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Stepper,
  StepperIndicator,
  StepperItem,
  StepperSeparator,
  StepperTitle,
  StepperTrigger,
} from '@/components/ui/stepper'
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
    </template>
  </PageShell>
</template>
