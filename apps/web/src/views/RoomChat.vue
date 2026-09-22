<script setup lang="ts">
import { computed, nextTick, onScopeDispose, ref, watch } from 'vue'
import { ArrowLeft, MessageSquare, Send, Timer, UserPlus, UserRound } from 'lucide-vue-next'
import { toast } from 'vue-sonner'

import { useRoomChat } from '@/composables/useRoomChat'
import { useAuth } from '@/composables/useAuth'
import { nextPhaseOf } from '@/domain/status'
import { senderLabel } from '@/domain/messages'
import { CANDIDATE_STATUSES, PERMISSIONS, type CandidateStatus } from '@/models'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { formatDateTime } from '@/lib/format'
import { toastError } from '@/lib/toast'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { IconBadge } from '@/components/ui/icon-badge'
import { Spinner } from '@/components/ui/spinner'
import { chatBubbleVariants } from '@/components/ui/tokens'
import EmptyState from '@/components/app/EmptyState.vue'
import { cn } from '@/lib/utils'

const props = defineProps<{ roomId: number }>()
const emit = defineEmits<{ back: [] }>()

// 组合式函数（函数式 ViewModel）：传 props.roomId，自动连接并在卸载时断开。
const {
  room,
  messages,
  connected,
  connecting,
  error,
  phase,
  pullPool,
  currentUserId,
  sendMessage,
  movePhase,
  pullCandidate,
} = useRoomChat(() => props.roomId)

const { hasPermission } = useAuth()

const draft = ref('')

/** 当前阶段允许推进到的下一档（纯函数推导，仅展示单步，完整状态机由后端校验）。 */
const nextPhase = computed<CandidateStatus | null>(() => nextPhaseOf(phase.value))

const hasCandidate = computed(() => !!room.value?.candidate)

// ---- 阶段计时器：待面试 / 面试中 显示已耗时（起点 = 候选人状态变更时间） ----
const TIMER_PHASES: readonly CandidateStatus[] = ['ASSIGNED', 'IN_PROGRESS']
const showTimer = computed(() => phase.value != null && TIMER_PHASES.includes(phase.value))
/** 计时起点（候选人 updated_at，状态机每次迁移都会刷新）。 */
const phaseStartedAt = computed(() => {
  const u = room.value?.candidate?.updated_at
  const t = u ? new Date(u).getTime() : NaN
  return Number.isFinite(t) ? t : null
})

const elapsedSeconds = ref(0)
let elapsedTimer: ReturnType<typeof setInterval> | null = null

watch(
  [showTimer, phaseStartedAt],
  ([on, start]) => {
    if (elapsedTimer) {
      clearInterval(elapsedTimer)
      elapsedTimer = null
    }
    if (on && start != null) {
      const tick = () => {
        elapsedSeconds.value = Math.max(0, Math.floor((Date.now() - start) / 1000))
      }
      tick()
      elapsedTimer = setInterval(tick, 1000)
    } else {
      elapsedSeconds.value = 0
    }
  },
  { immediate: true },
)
onScopeDispose(() => {
  if (elapsedTimer) clearInterval(elapsedTimer)
})

const pad2 = (n: number) => String(n).padStart(2, '0')
const elapsedLabel = computed(() => {
  const s = elapsedSeconds.value
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  return h > 0 ? `${h}:${pad2(m)}:${pad2(s % 60)}` : `${pad2(m)}:${pad2(s % 60)}`
})

function send() {
  const text = draft.value.trim()
  if (!text) return
  sendMessage(text)
  draft.value = ''
}

async function advance() {
  if (!nextPhase.value) return
  movePhase(nextPhase.value)
}

async function handlePull(candidateId: number) {
  try {
    await pullCandidate(candidateId)
    toast.success('候选人已拉入房间')
  } catch (e) {
    toastError(e)
  }
}

// ---- 消息区自动滚动：跟随新消息贴底；用户上翻阅读历史时不打断。 ----
const scrollBox = ref<HTMLElement | null>(null)
/** 视口是否处于底部附近（48px 容差）。 */
const atBottom = ref(true)

function onScroll() {
  const el = scrollBox.value
  if (!el) return
  atBottom.value = el.scrollHeight - el.scrollTop - el.clientHeight < 48
}

async function scrollToBottom() {
  await nextTick()
  const el = scrollBox.value
  if (el) el.scrollTop = el.scrollHeight
}

// 新消息到达且视口在底部 → 自动贴底。
watch(
  () => messages.value.length,
  (len, prev) => {
    if (len > (prev ?? 0) && atBottom.value) void scrollToBottom()
  },
)

// 连接建立完成（首帧同步到达）→ 贴底展示最新消息。
watch(connecting, (v, prev) => {
  if (prev && !v) void scrollToBottom()
})

// ---- 状态机步骤条：当前档高亮，已完成档打勾填充。 ----
const phaseIndex = computed(() =>
  phase.value ? CANDIDATE_STATUSES.indexOf(phase.value) : -1,
)
</script>

<template>
  <div class="relative mx-auto flex h-full w-full max-w-content flex-col overflow-hidden">
    <!-- 悬浮胶囊：返回键 + 房间号 + 连接状态 + 阶段计时器（顶栏下方左侧） -->
    <div
      class="absolute left-4 top-4 z-30 flex h-9 w-72 items-center gap-2 rounded-md border bg-background/95 py-1 pl-1 pr-3 text-xs text-muted-foreground shadow-sm backdrop-blur"
    >
      <Button
        class="h-7 w-7 shrink-0"
        size="icon"
        variant="ghost"
        aria-label="返回房间列表"
        @click="emit('back')"
      >
        <ArrowLeft class="h-4 w-4" />
      </Button>
      <span class="h-4 w-px shrink-0 bg-border" aria-hidden="true" />
      <span class="min-w-0 flex-1 truncate font-semibold text-foreground">房间 #{{ props.roomId }}</span>
      <Spinner v-if="connecting" class="h-3.5 w-3.5 shrink-0" />
      <!-- 连接状态点：语义色 token（success/destructive）。 -->
      <span
        v-else
        class="inline-block h-2 w-2 shrink-0 rounded-full"
        :class="connected ? 'bg-success' : 'bg-destructive'"
        aria-hidden="true"
      ></span>
      <span role="status" class="shrink-0">{{ connecting ? '连接中…' : connected ? '已连接' : '未连接' }}</span>
      <!-- 阶段计时器：待面试 / 面试中 显示已耗时 -->
      <template v-if="showTimer">
        <span class="h-4 w-px shrink-0 bg-border" aria-hidden="true" />
        <span
          class="flex shrink-0 items-center gap-1.5 font-medium tabular-nums text-foreground"
          role="timer"
          :aria-label="`本阶段已进行 ${elapsedLabel}`"
        >
          <Timer class="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
          {{ elapsedLabel }}
        </span>
      </template>
    </div>
    <!-- 连接错误提示 -->
    <div
      v-if="error"
      role="alert"
      class="shrink-0 border-t border-destructive/40 bg-destructive/10 px-4 py-2 text-sm text-destructive"
    >
      {{ error }}
    </div>

    <!-- 主区域：聊天 + 侧栏（小屏上下堆叠） -->
    <div class="flex min-h-0 min-w-0 flex-1 flex-col lg:flex-row">
      <aside
        class="order-last flex w-full shrink-0 flex-col border-t lg:order-first lg:h-full lg:w-80 lg:border-t-0"
      >
        <div class="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 pb-4 pt-16">
          <!-- 候选人信息 -->
          <Card>
            <CardContent class="space-y-3 p-4">
              <div class="flex items-center gap-3">
                <IconBadge size="md" tone="primary" :class="hasCandidate ? '' : 'bg-muted text-muted-foreground'">
                  <UserRound aria-hidden="true" />
                </IconBadge>
                <div class="min-w-0">
                  <p class="truncate font-medium" :class="hasCandidate ? '' : 'text-muted-foreground'">
                    {{ room?.candidate?.name ?? '空房' }}
                  </p>
                  <p class="text-xs text-muted-foreground">{{ hasCandidate ? '候选人' : '等待拉取候选人' }}</p>
                </div>
              </div>
              <div class="space-y-1 text-sm">
                <div class="flex justify-between">
                  <span class="text-muted-foreground">房间</span>
                  <span class="font-mono">#{{ props.roomId }}</span>
                </div>
                <div v-if="room?.candidate?.profile" class="flex justify-between gap-3">
                  <span class="shrink-0 text-muted-foreground">简介</span>
                  <span class="whitespace-pre-line break-words text-right">{{ room.candidate.profile }}</span>
                </div>
              </div>
            </CardContent>
          </Card>

          <!-- 拉取候选人 -->
          <Card v-if="hasPermission(PERMISSIONS.CANDIDATES_ASSIGN) && !hasCandidate">
            <CardContent class="space-y-3 p-4">
              <p class="flex items-center gap-2 text-sm font-medium">
                <UserPlus class="h-4 w-4" aria-hidden="true" />
                拉取候选人
              </p>
              <EmptyState
                v-if="pullPool.length === 0"
                bare
                :icon="UserRound"
                class="py-6"
              >
                暂无已签到的候选人
              </EmptyState>
              <ul v-else class="space-y-2">
                <li
                  v-for="c in pullPool"
                  :key="c.id"
                  class="flex items-center justify-between gap-2 rounded-md bg-muted p-2"
                >
                  <span class="min-w-0 truncate text-sm">
                    {{ c.name }}
                    <span v-if="c.profile" class="text-xs text-muted-foreground">· {{ c.profile }}</span>
                  </span>
                  <Button size="sm" variant="outline" @click="handlePull(c.id)">拉取</Button>
                </li>
              </ul>
            </CardContent>
          </Card>

          <!-- 状态 + 阶段控制 -->
          <Card v-if="hasCandidate">
            <CardContent class="space-y-3 p-4">
              <div class="flex items-center justify-between">
                <p class="text-sm font-medium">当前状态</p>
                <Badge v-if="phase" :variant="STATUS_PRESENTATION[phase].badge">
                  {{ STATUS_PRESENTATION[phase].label }}
                </Badge>
                <Badge v-else variant="outline">—</Badge>
              </div>

              <!-- 状态机竖向步骤条：已完成实心、当前高亮、未达置灰 -->
              <ol class="flex flex-col" aria-label="面试状态机进度">
                <li
                  v-for="(s, i) in CANDIDATE_STATUSES"
                  :key="s"
                  class="flex gap-3"
                >
                  <!-- 节点列：圆点 + 连接线 -->
                  <div class="flex flex-col items-center">
                    <span
                      class="mt-1 flex h-3 w-3 shrink-0 items-center justify-center rounded-full border-2"
                      :class="
                        i < phaseIndex
                          ? 'border-primary bg-primary'
                          : i === phaseIndex
                            ? 'border-primary bg-background ring-4 ring-primary/15'
                            : 'border-muted-foreground/30 bg-transparent'
                      "
                      aria-hidden="true"
                    />
                    <span
                      v-if="i < CANDIDATE_STATUSES.length - 1"
                      class="min-h-4 w-0.5 flex-1"
                      :class="i < phaseIndex ? 'bg-primary' : 'bg-border'"
                      aria-hidden="true"
                    />
                  </div>
                  <!-- 标签列 -->
                  <span
                    class="pb-4 text-xs leading-none"
                    :class="
                      i === phaseIndex
                        ? 'font-semibold text-foreground'
                        : i < phaseIndex
                          ? 'text-muted-foreground'
                          : 'text-muted-foreground/60'
                    "
                    :aria-current="i === phaseIndex ? 'step' : undefined"
                  >
                    {{ STATUS_PRESENTATION[s].label }}
                  </span>
                </li>
              </ol>

              <Button
                v-if="hasPermission(PERMISSIONS.ROOMS_MOVE_PHASE)"
                class="w-full"
                :disabled="!nextPhase || connecting"
                @click="advance"
              >
                推进到「{{ nextPhase ? STATUS_PRESENTATION[nextPhase].label : '—' }}」
              </Button>
            </CardContent>
          </Card>
        </div>
      </aside>
      <!-- 聊天面板：定高（撑满剩余空间，内部滚动），有边框浮动卡片 -->
      <div class="flex min-w-0 flex-1 flex-col p-4">
        <div class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border bg-background">
        <!-- 消息滚动区：role=log + aria-live 便于读屏播报新消息 -->
        <div
          ref="scrollBox"
          class="min-h-0 flex-1 space-y-3 overflow-y-auto p-4"
          role="log"
          aria-live="polite"
          @scroll.passive="onScroll"
        >
          <!-- 空房/无消息空态（bare：直接置于聊天滚动区，无卡片包裹） -->
          <EmptyState v-if="!hasCandidate && !connecting" bare :icon="UserRound" class="py-12">
            房间空闲，从左侧「拉取候选人」开始面试
          </EmptyState>
          <EmptyState v-else-if="!messages.length && !connecting" bare :icon="MessageSquare" class="py-12">
            暂无消息，发送第一条面试记录吧
          </EmptyState>

          <div
            v-for="m in messages"
            :key="m.id"
            class="flex max-w-full flex-col gap-1"
            :class="m.sender_id === currentUserId ? 'items-end' : 'items-start'"
          >
            <div class="flex items-baseline gap-2 px-1 text-xs text-muted-foreground">
              <span class="font-medium">{{ senderLabel(m.sender_id, currentUserId, m.sender?.name || m.sender?.username) }}</span>
              <time>{{ formatDateTime(m.created_at) }}</time>
            </div>
            <div :class="cn(chatBubbleVariants({ side: m.sender_id === currentUserId ? 'own' : 'other' }))">
              {{ m.content }}
            </div>
          </div>
        </div>

        <!-- 输入区 -->
        <div class="flex shrink-0 items-center gap-2 border-t p-3">
          <Input
            v-model="draft"
            :disabled="!hasCandidate"
            :placeholder="hasCandidate ? '输入面试记录，Enter 发送…' : '等待候选人进房后可发送消息'"
            aria-label="消息内容"
            @keydown.enter.prevent="send"
          />
          <Button size="icon" aria-label="发送消息" :disabled="!draft.trim() || !hasCandidate" @click="send">
            <Send aria-hidden="true" />
          </Button>
        </div>
        </div>
      </div>

    </div>
  </div>
</template>
