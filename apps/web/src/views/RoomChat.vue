<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowLeft, ChevronRight, Send, UserRound } from 'lucide-vue-next'

import { useRoomChat } from '@/composables/useRoomChat'
import { nextPhaseOf } from '@/domain/status'
import { CANDIDATE_STATUSES, type CandidateStatus } from '@/models'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { formatDateTime } from '@/lib/format'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'

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
  currentUserId,
  sendMessage,
  movePhase,
} = useRoomChat(() => props.roomId)

const draft = ref('')

/** 当前阶段允许推进到的下一档（纯函数推导，仅展示单步，完整状态机由后端校验）。 */
const nextPhase = computed<CandidateStatus | null>(() => nextPhaseOf(phase.value))

function senderLabel(senderId: number): string {
  if (senderId === currentUserId) return `我 (${currentUserId})`
  return `面试官 ${senderId}`
}

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

// 消息区自动滚动到底
const msgBox = ref<HTMLElement | null>(null)
watch(
  () => messages.value.length,
  () => {
    if (msgBox.value) msgBox.value.scrollTop = msgBox.value.scrollHeight
  },
)
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden">
    <!-- 房间头部：返回键 + 房间名 -->
    <header class="flex h-14 shrink-0 items-center gap-3 border-b px-4">
      <Button
        variant="ghost"
        size="icon"
        aria-label="返回房间列表"
        @click="emit('back')"
      >
        <ArrowLeft class="h-5 w-5" />
      </Button>
      <h1 class="text-base font-semibold tracking-tight">房间 #{{ props.roomId }}</h1>
      <span class="ml-auto flex items-center gap-2 text-xs text-muted-foreground">
        <span
          class="inline-block h-2 w-2 rounded-full"
          :class="connected ? 'bg-emerald-500' : 'bg-slate-300'"
        />
        {{ connected ? '已连接' : '未连接' }}
      </span>
    </header>

    <!-- 连接错误提示 -->
    <div
      v-if="error"
      class="shrink-0 border-t border-destructive/40 bg-destructive/10 px-4 py-2 text-sm text-destructive"
    >
      {{ error }}
    </div>

    <!-- 主区域：聊天 + 侧栏 -->
    <div class="flex min-h-0 flex-1">
      <!-- 聊天区 -->
      <div class="flex min-w-0 flex-1 flex-col">
        <!-- 消息滚动区 -->
        <div
          ref="msgBox"
          class="min-h-0 flex-1 space-y-3 overflow-y-auto p-4"
        >
          <div v-if="!messages.length && !connecting" class="py-12 text-center text-muted-foreground">
            暂无消息，发送第一条吧。
          </div>
          <div v-if="connecting">
            <div class="space-y-3">
              <Skeleton v-for="i in 3" :key="i" class="h-12 w-full" />
            </div>
          </div>
          <div
            v-for="m in messages"
            :key="m.id"
            class="flex flex-col gap-1"
            :class="m.sender_id === currentUserId ? 'items-end' : 'items-start'"
          >
            <div class="text-xs text-muted-foreground">
              {{ senderLabel(m.sender_id) }} · {{ formatDateTime(m.created_at) }}
            </div>
            <div
              class="max-w-[75%] whitespace-pre-wrap rounded-lg px-3 py-2 text-sm"
              :class="
                m.sender_id === currentUserId
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-muted'
              "
            >
              {{ m.content }}
            </div>
          </div>
        </div>
        <!-- 输入区 -->
        <div class="flex shrink-0 items-center gap-2 border-t p-3">
          <Input
            v-model="draft"
            placeholder="输入面试记录…"
            @keydown.enter.prevent="send"
          />
          <Button size="icon" @click="send" :disabled="!draft.trim()">
            <Send class="h-4 w-4" />
          </Button>
        </div>
      </div>

      <!-- 侧栏：面试人信息 + 状态 -->
      <aside class="flex w-72 shrink-0 flex-col border-l bg-muted/20">
        <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
          <!-- 面试人信息 -->
          <Card>
            <CardContent class="space-y-3 p-4">
              <div class="flex items-center gap-3">
                <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10 text-primary">
                  <UserRound class="h-5 w-5" />
                </div>
                <div class="min-w-0">
                  <p class="truncate font-medium">
                    {{ room?.candidate?.name ?? '未绑定候选人' }}
                  </p>
                  <p class="text-xs text-muted-foreground">候选人</p>
                </div>
              </div>
              <div class="space-y-1 text-sm">
                <div class="flex justify-between">
                  <span class="text-muted-foreground">房间</span>
                  <span class="font-mono">#{{ props.roomId }}</span>
                </div>
                <div v-if="room?.candidate?.profile" class="flex justify-between gap-2">
                  <span class="shrink-0 text-muted-foreground">简介</span>
                  <span class="truncate text-right">{{ room.candidate.profile }}</span>
                </div>
              </div>
            </CardContent>
          </Card>

          <!-- 状态 + 阶段控制 -->
          <Card>
            <CardContent class="space-y-3 p-4">
              <div class="flex items-center justify-between">
                <p class="text-sm font-medium">当前状态</p>
                <Badge v-if="phase" :variant="STATUS_PRESENTATION[phase].badge">
                  {{ STATUS_PRESENTATION[phase].label }}
                </Badge>
                <Badge v-else variant="outline">—</Badge>
              </div>

              <!-- 状态机进度 -->
              <div class="rounded-md bg-muted p-3 text-xs text-muted-foreground">
                <p class="mb-2 font-medium text-foreground">状态机</p>
                <ol class="space-y-1">
                  <li
                    v-for="(s, i) in CANDIDATE_STATUSES"
                    :key="s"
                    class="flex items-center gap-1"
                  >
                    <span v-if="i > 0"><ChevronRight class="h-3 w-3" /></span>
                    <span :class="phase === s ? 'font-semibold text-foreground' : ''">
                      {{ STATUS_PRESENTATION[s].label }}
                    </span>
                  </li>
                </ol>
              </div>

              <Button
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
    </div>
  </div>
</template>
