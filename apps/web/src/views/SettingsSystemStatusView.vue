<!--
  SettingsSystemStatusView —— 系统状态设置：在「面试阶段 / 录取阶段」之间切换。
  段式选择控件的实底高亮即当前阶段；点击另一档即切换。
-->
<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { toast } from 'vue-sonner'
import { RefreshCw, UserCheck, UsersRound } from 'lucide-vue-next'

import { useSystemStatus } from '@/composables/useSystemStatus'
import type { SystemPhase } from '@/models'
import { SYSTEM_PHASES } from '@/models'

import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { tabItemVariants } from '@/components/ui/tokens'
import PageShell from '@/components/app/PageShell.vue'

const { status, loading, load, setPhase } = useSystemStatus()
const switching = ref(false)

onMounted(() => void load())

const phaseMeta: Record<SystemPhase, { label: string; icon: typeof UsersRound }> = {
  interview: { label: '面试阶段', icon: UsersRound },
  admission: { label: '录取阶段', icon: UserCheck },
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
        <div class="flex items-center gap-1 rounded-lg p-1" role="group" aria-label="系统阶段">
          <button
            v-for="p in SYSTEM_PHASES"
            :key="p"
            type="button"
            class="flex flex-1 items-center justify-center gap-2"
            :class="tabItemVariants({ active: status?.phase === p })"
            :aria-pressed="status?.phase === p"
            :disabled="switching"
            @click="switchTo(p)"
          >
            <component :is="phaseMeta[p].icon" class="h-4 w-4 shrink-0" aria-hidden="true" />
            {{ phaseMeta[p].label }}
          </button>
        </div>
      </Card>
    </template>
  </PageShell>
</template>