<!--
  WaitingBoardView —— 候场大屏：未完成名单 + 签到操作。
  面向现场投屏：按状态机四档分栏（待签到 / 排队中 / 待面试 / 面试中），
  大字号、高密度、自动轮询刷新（10s，页面隐藏时暂停）。
  签到按钮仅对持有 candidates.checkin 权限的用户可用；名单对任意登录用户可见。
-->
<script setup lang="ts">
import { computed, onMounted, onScopeDispose, ref } from 'vue'
import { toast } from 'vue-sonner'
import { RefreshCw } from 'lucide-vue-next'

import { candidateApi } from '@/api/http'
import { groupWaitingColumns, type WaitingColumn } from '@/domain/status'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { PERMISSIONS, type Candidate } from '@/models'
import { useAuth } from '@/composables/useAuth'
import { formatDateTime } from '@/lib/format'

import { Button } from '@/components/ui/button'
import EmptyState from '@/components/app/EmptyState.vue'

const { hasPermission } = useAuth()

const canCheckin = computed(() => hasPermission(PERMISSIONS.CANDIDATES_CHECKIN))

const candidates = ref<Candidate[]>([])
const loading = ref(false)
const error = ref('')
/** 上次成功刷新时间（大屏自证数据新鲜度）。 */
const lastUpdated = ref<string>('')

async function load(silent = false) {
  if (loading.value) return
  if (!silent) loading.value = true
  try {
    const res = await candidateApi.list({ limit: 200 })
    candidates.value = res.items
    lastUpdated.value = formatDateTime(new Date().toISOString())
    error.value = ''
  } catch (e) {
    // 静默轮询失败不打断展示，仅记录错误条（有数据时仍显示旧名单）。
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

// ---- 自动轮询：10s 一次；页面隐藏时暂停，恢复可见立即刷新。 ----
// 注意：onScopeDispose 须在 setup 同步期注册，定时器在 onMounted 里创建。
const POLL_MS = 10_000
let pollTimer = 0
const onVisibilityChange = () => {
  if (!document.hidden) void load(true)
}

onMounted(() => {
  void load()
  pollTimer = window.setInterval(() => {
    if (!document.hidden) void load(true)
  }, POLL_MS)
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onScopeDispose(() => {
  window.clearInterval(pollTimer)
  document.removeEventListener('visibilitychange', onVisibilityChange)
})

/** 未完成名单分栏（COMPLETED 不上屏）。 */
const columns = computed<WaitingColumn[]>(() => groupWaitingColumns(candidates.value))

const waitingCount = computed(() => columns.value.reduce((n, col) => n + col.candidates.length, 0))

async function handleCheckin(c: { id: number; name: string }) {
  try {
    await candidateApi.checkin(c.id)
    toast.success(`「${c.name}」已签到`)
    await load(true)
  } catch (e) {
    toast.error((e as Error).message)
  }
}
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden">
    <!-- 大屏头部 -->
    <header class="flex shrink-0 items-center gap-3 border-b px-4 py-3">
      <h1 class="text-xl font-semibold tracking-tight sm:text-2xl">候场大屏</h1>
      <span class="rounded-md bg-primary px-2 py-0.5 text-sm font-semibold text-primary-foreground">
        {{ waitingCount }}
      </span>
      <div class="ml-auto flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
        <span v-if="lastUpdated" title="每 10 秒自动刷新">更新于 {{ lastUpdated }}</span>
        <span v-if="error" role="alert" class="max-w-[16rem] truncate text-destructive">{{ error }}</span>
        <Button variant="outline" size="icon" aria-label="立即刷新" @click="load()">
          <RefreshCw :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
        </Button>
      </div>
    </header>

    <!-- 分栏名单 -->
    <div class="min-h-0 flex-1 overflow-y-auto p-4">
      <EmptyState v-if="!loading && candidates.length === 0" bare class="py-20">
        暂无候选人
        <template #hint>请在「候选人管理」新增候选人后签到</template>
      </EmptyState>

      <div v-else class="grid gap-4 lg:grid-cols-2 xl:grid-cols-4">
        <section
          v-for="col in columns"
          :key="col.status"
          class="flex flex-col overflow-hidden rounded-xl border bg-card"
          :aria-label="STATUS_PRESENTATION[col.status].label"
        >
          <!-- 栏头：状态名 + 计数 -->
          <header class="flex items-center justify-between border-b bg-muted/40 px-4 py-3">
            <h2 class="text-base font-semibold tracking-tight sm:text-lg">
              {{ STATUS_PRESENTATION[col.status].label }}
            </h2>
            <span class="min-w-6 rounded-full bg-secondary px-2 py-0.5 text-center text-sm font-semibold text-secondary-foreground">
              {{ col.candidates.length }}
            </span>
          </header>

          <!-- 名单 -->
          <ul v-if="col.candidates.length" class="flex-1 space-y-2 p-3">
            <li
              v-for="(c, i) in col.candidates"
              :key="c.id"
              class="flex items-center gap-3 rounded-lg border p-3"
              :class="col.status === 'NOT_CHECKED_IN' ? '' : 'bg-muted/30'"
            >
              <span
                class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-semibold text-primary"
                aria-hidden="true"
              >
                {{ i + 1 }}
              </span>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-base font-medium">{{ c.name }}</span>
                <span v-if="c.profile" class="block truncate text-xs text-muted-foreground">{{ c.profile }}</span>
              </span>
              <Button
                v-if="col.status === 'NOT_CHECKED_IN' && canCheckin"
                size="sm"
                :aria-label="`为 ${c.name} 签到`"
                @click="handleCheckin(c)"
              >
                签到
              </Button>
            </li>
          </ul>

          <!-- 空栏占位 -->
          <p v-else class="flex flex-1 items-center justify-center py-8 text-sm text-muted-foreground">—</p>
        </section>
      </div>
    </div>
  </div>
</template>
