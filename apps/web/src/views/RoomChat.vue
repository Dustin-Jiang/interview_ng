<script setup lang="ts">
import { computed, nextTick, onScopeDispose, ref, watch } from 'vue'
import { ArrowDown, ArrowLeft, MessageSquare, Timer, UserRound } from 'lucide-vue-next'
import { toast } from 'vue-sonner'

import { useRoomChat } from '@/composables/useRoomChat'
import { useWaitingQueue } from '@/composables/useWaitingQueue'
import { nextPhaseOf, isInProgress } from '@/domain/status'
import { roomLabel } from '@/domain/room'
import type { CandidateStatus } from '@/models'
import { formatDateTime } from '@/lib/format'
import { toastError } from '@/lib/toast'

import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import EmptyState from '@/components/app/EmptyState.vue'
import MessageComposer from '@/components/app/MessageComposer.vue'
import MessageTranscript from '@/components/app/MessageTranscript.vue'
import RoomSidebar from '@/components/app/RoomSidebar.vue'

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
  sendMessage,
  movePhase,
  pullCandidate,
  reloadRoom,
} = useRoomChat(() => props.roomId)

// 拉取池 = 候场队列：唯一共享数据源（useWaitingQueue，按看板通道事件刷新，
// 与候场大屏同一份顺序口径），不再由 useRoomChat 各自拉一份。
const { queue: pullPool, reload: reloadPullPool } = useWaitingQueue()

const draft = ref('')

/** 当前阶段允许推进到的下一档（纯函数推导，仅展示单步，完整状态机由后端校验）。 */
const nextPhase = computed<CandidateStatus | null>(() => nextPhaseOf(phase.value))

const hasCandidate = computed(() => !!room.value?.candidate)

/**
 * 消息通道是否开放：只有「面试中」（IN_PROGRESS）的候选人可写记录。
 * 「待面试」（已分配进房、还没点开始面试）与面试结档（已完成及其后的录取档，此时后端已解绑房间）
 * 都发不了——前者先推进到「面试中」，后者的记录转为只读归档（与后端 AppendMessage 同一判据）。
 */
const canSend = computed(() => isInProgress(phase.value))

/** 「还不能发记录」的原因：区分「房间空闲」「还没开始面试」「面试已结束」三种。 */
const blockedReason = computed(() => {
  if (!hasCandidate.value) return '等待候选人进房后可发送消息'
  if (phase.value === 'ASSIGNED') return '尚未开始面试，推进到「面试中」后即可记录'
  return '面试已结束，面试记录只读'
})

/** 输入框占位文案。 */
const inputPlaceholder = computed(() =>
  canSend.value ? '输入面试记录，Enter 发送…' : blockedReason.value,
)

/** 无消息时的空态文案：还没开始时提示怎么开始，结档后说明记录只读。 */
const emptyHint = computed(() =>
  canSend.value ? '暂无消息，发送第一条面试记录吧' : blockedReason.value,
)

// ---- 阶段计时器：仅在「面试中」计时，起点 = 切到面试中的那一刻 ----
const showTimer = computed(() => phase.value === 'IN_PROGRESS')

/**
 * 本端观测到的「切到面试中」时刻。
 * 房间快照里的 candidate.interview_started_at 会被后续阶段事件沿用旧值（phaseRoom 只改 status），
 * 因此只有首帧快照（未观测到迁移）才退回后端打点。
 */
const observedStart = ref<number | null>(null)
/** 首帧快照装载的状态不算「切换」，否则中途打开页面会把计时清零。 */
let phaseSeen = false

watch(() => phase.value, (now) => {
  if (!phaseSeen) {
    phaseSeen = true
    return
  }
  observedStart.value = now === 'IN_PROGRESS' ? Date.now() : null
})

/** 计时起点：优先本端观测时刻，其次后端打点 interview_started_at（页面在面试中途打开）。 */
const phaseStartedAt = computed(() => {
  if (observedStart.value != null) return observedStart.value
  const stamped = room.value?.candidate?.interview_started_at
  const t = stamped ? new Date(stamped).getTime() : NaN
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
  // 自己发完就跳到最新：上翻读历史时发送，不该停在原处看不见自己刚写的那条。
  void scrollToBottom()
}

async function advance() {
  if (!nextPhase.value) return
  movePhase(nextPhase.value)
}

async function handlePull(candidateId: number) {
  try {
    await pullCandidate(candidateId)
    // 自己拉的立即对齐（其余端由 candidate_assigned 事件刷新）：被拉者要马上从队列里消失。
    void reloadPullPool()
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
</script>

<template>
  <div class="relative mx-auto flex h-full w-full max-w-content flex-col overflow-hidden">
    <!-- 返回键 + 房间号 + 连接状态 + 阶段计时器。
         <lg 布局为纵向堆叠：绝对定位会压住聊天首条消息，且 w-72 胶囊在 360px 下会溢出，
         故此处内联为页头一条；≥lg 布局左右分栏，恢复悬浮胶囊（叠在左侧栏上方）。 -->
    <div
      class="z-30 flex min-h-11 w-full shrink-0 items-center gap-2 border-b bg-background/95 px-2 text-xs text-muted-foreground lg:absolute lg:left-4 lg:top-4 lg:min-h-0 lg:w-72 lg:max-w-[calc(100vw-2rem)] lg:rounded-md lg:border lg:py-1 lg:pl-1 lg:pr-3 lg:shadow-sm lg:backdrop-blur"
    >
      <Button
        class="h-7 w-7 shrink-0 max-lg:h-11 max-lg:w-11"
        size="icon"
        variant="ghost"
        aria-label="返回房间列表"
        @click="emit('back')"
      >
        <ArrowLeft class="h-4 w-4" />
      </Button>
      <span class="h-4 w-px shrink-0 bg-border" aria-hidden="true" />
      <span class="min-w-0 flex-1 truncate font-semibold text-foreground">{{ roomLabel(room) }}</span>
      <Spinner v-if="connecting" class="h-3.5 w-3.5 shrink-0" />
      <!-- 连接状态点：语义色 token（success/destructive）。 -->
      <span
        v-else
        class="inline-block h-2 w-2 shrink-0 rounded-full"
        :class="connected ? 'bg-success' : 'bg-destructive'"
        aria-hidden="true"
      ></span>
      <span role="status" class="shrink-0">{{ connecting ? '连接中…' : connected ? '已连接' : '未连接' }}</span>
      <!-- 阶段计时器：面试中显示已耗时 -->
      <template v-if="showTimer">
        <span class="h-4 w-px shrink-0 bg-border" aria-hidden="true" />
        <span
          class="flex shrink-0 items-center gap-1.5 font-medium tabular-nums text-foreground"
          role="timer"
          :aria-label="`面试已进行 ${elapsedLabel}`"
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
      <RoomSidebar
        :room="room"
        :phase="phase"
        :next-phase="nextPhase"
        :pull-pool="pullPool"
        :connecting="connecting"
        @pull="handlePull"
        @advance="advance"
        @saved="reloadRoom"
      />

      <!-- 聊天面板：定高（撑满剩余空间，内部滚动），有边框浮动卡片。
           min-h-0 必须保留：软键盘弹出时 dvh 收缩，优先压缩消息区，输入区不挤出视口。 -->
      <div class="flex min-h-0 min-w-0 flex-1 flex-col p-2 md:p-4">
        <div class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border bg-background">
        <!-- 消息区：滚动容器（role=log + aria-live 便于读屏播报新消息）+ 浮在其上的「回到最新」。
             浮标刻意放在 live region 之外：读屏不该把「回到最新」当新消息播报。 -->
        <div class="relative flex min-h-0 flex-1 flex-col">
          <div
            ref="scrollBox"
            class="min-h-0 flex-1 space-y-3 overflow-y-auto p-4"
            role="log"
            aria-live="polite"
            @scroll.passive="onScroll"
          >
            <!-- 空房/无消息空态（bare：直接置于聊天滚动区，无卡片包裹） -->
            <EmptyState v-if="!hasCandidate && !connecting" bare :icon="UserRound" class="py-12">
              房间空闲，拉取候选人后开始面试
            </EmptyState>
            <EmptyState v-else-if="!messages.length && !connecting" bare :icon="MessageSquare" class="py-12">
              {{ emptyHint }}
            </EmptyState>

            <!-- 消息列表：与归档页共用 MessageTranscript（同一视觉语言 + 同一套编辑/撤回交互） -->
            <MessageTranscript v-else :messages="messages" @changed="reloadRoom" />

          </div>

          <!-- 上翻读历史时给一个回到最新的浮标（MessageScroller 的 jump-to-latest）：浮在消息区底部，不常驻 -->
          <div
            v-if="!atBottom && !connecting && messages.length"
            class="pointer-events-none absolute inset-x-0 bottom-2 z-10 flex justify-center"
          >
            <Button size="sm" variant="secondary" class="pointer-events-auto gap-1 shadow-sm" @click="scrollToBottom">
              <ArrowDown aria-hidden="true" />
              回到最新
            </Button>
          </div>
        </div>

        <!-- 输入区：shrink-0 保证软键盘弹出时输入框与发送键始终可见；输入框 min-w-0 允许被压缩不留溢出。
             输入区与归档页的「补充记录」共用 `MessageComposer`。 -->
        <div class="shrink-0 border-t p-3">
          <MessageComposer
            v-model="draft"
            :disabled="!canSend"
            :placeholder="inputPlaceholder"
            @send="send"
          />
        </div>
        </div>
      </div>

    </div>
  </div>
</template>
