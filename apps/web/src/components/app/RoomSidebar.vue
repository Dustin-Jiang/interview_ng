<!--
  RoomSidebar —— 面试房间左侧信息栏（RoomChat 专用，三张卡片 + 简介卡片）。
  候选人信息（学号 / 志愿 / 接受调剂 / 联系方式）、个人简介单独成卡（长文本不撑破信息行）、
  拉取候选人、当前状态与阶段控制。
  纯展示 + 向上抛事件：数据与命令仍由 RoomChat 的 useRoomChat 持有。
-->
<script setup lang="ts">
import { computed, ref } from 'vue'
import { Pencil, UserPlus, UserRound } from 'lucide-vue-next'

import { useAuth } from '@/composables/useAuth'
import {
  CANDIDATE_STATUSES,
  PERMISSIONS,
  type Candidate,
  type CandidateStatus,
  type Room,
} from '@/models'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { roomLabel } from '@/domain/room'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { IconBadge } from '@/components/ui/icon-badge'
import CandidatePreferenceDialog from '@/components/app/CandidatePreferenceDialog.vue'
import EmptyState from '@/components/app/EmptyState.vue'

const props = defineProps<{
  room: Room | null
  phase: CandidateStatus | null
  /** 当前阶段允许推进到的下一档（null = 无下一步）。 */
  nextPhase: CandidateStatus | null
  /** 可拉取的已签到候选人。 */
  pullPool: readonly Candidate[]
  connecting: boolean
}>()

const emit = defineEmits<{ pull: [candidateId: number]; advance: []; saved: [] }>()

const { hasPermission } = useAuth()

const hasCandidate = computed(() => !!props.room?.candidate)

// ---- 志愿与调剂编辑（独立小权限；保存后由父级重拉房间快照） ----
const canEditPreferences = computed(() => hasPermission(PERMISSIONS.CANDIDATES_PREFERENCES))
const preferenceTarget = ref<Candidate | null>(null)

function openPreferences(): void {
  const c = props.room?.candidate
  if (c) preferenceTarget.value = c
}

/** 状态机步骤条：当前档在 CANDIDATE_STATUSES 中的索引（无候选人为 -1）。 */
const phaseIndex = computed(() =>
  props.phase ? CANDIDATE_STATUSES.indexOf(props.phase) : -1,
)
</script>

<template>
  <!-- 纵向堆叠（<lg：手机 / 平板竖屏）时若不限高，内部 overflow-y-auto 形同失效，
       侧栏会撑满内容高度把底部「推进阶段」按钮挤出视口；此处限高 45dvh 使其内部可滚动。
       ≥lg 左右分栏后由 h-full 撑满；pt-16 只为 ≥lg 的悬浮胶囊让位。 -->
  <aside
    class="order-last flex w-full shrink-0 flex-col border-t max-lg:max-h-[45dvh] lg:order-first lg:h-full lg:w-80 lg:border-t-0"
  >
    <div class="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 pb-4 pt-4 lg:pt-16">
      <!-- 候选人信息 -->
      <Card>
        <CardContent class="space-y-3 p-4">
          <div class="flex items-start gap-3">
            <IconBadge size="md" tone="primary" :class="hasCandidate ? '' : 'bg-muted text-muted-foreground'">
              <UserRound aria-hidden="true" />
            </IconBadge>
            <div class="min-w-0 flex-1">
              <p class="truncate font-medium" :class="hasCandidate ? '' : 'text-muted-foreground'">
                {{ room?.candidate?.name ?? '空房' }}
              </p>
              <p class="text-xs text-muted-foreground">{{ hasCandidate ? '候选人' : '等待拉取候选人' }}</p>
            </div>
            <Button
              v-if="canEditPreferences && hasCandidate"
              size="icon"
              variant="ghost"
              class="h-7 w-7 shrink-0 text-muted-foreground max-lg:h-11 max-lg:w-11"
              aria-label="修改志愿与调剂"
              @click="openPreferences"
            >
              <Pencil class="h-4 w-4" />
            </Button>
          </div>
          <div class="space-y-1 text-sm">
            <div class="flex justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">房间</span>
              <span class="min-w-0 break-words text-right">{{ roomLabel(props.room) }}</span>
            </div>
            <div v-if="room?.candidate?.student_no" class="flex justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">学号</span>
              <span class="tabular-nums">{{ room.candidate.student_no }}</span>
            </div>
            <div v-if="room?.candidate?.first_choice" class="flex justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">第一志愿</span>
              <span class="min-w-0 break-words text-right">{{ room.candidate.first_choice }}</span>
            </div>
            <div v-if="room?.candidate?.second_choice" class="flex justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">第二志愿</span>
              <span class="min-w-0 break-words text-right">{{ room.candidate.second_choice }}</span>
            </div>
            <div
              v-if="room?.candidate && (room.candidate.first_choice || room.candidate.second_choice)"
              class="flex justify-between gap-3"
            >
              <span class="shrink-0 text-muted-foreground">接受调剂</span>
              <span>{{ room.candidate.accept_adjust ? '接受' : '不接受' }}</span>
            </div>
            <div v-if="room?.candidate?.phone" class="flex justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">手机号</span>
              <span class="min-w-0 break-all text-right tabular-nums">{{ room.candidate.phone }}</span>
            </div>
            <div v-if="room?.candidate?.qq" class="flex justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">QQ号</span>
              <span class="min-w-0 break-all text-right tabular-nums">{{ room.candidate.qq }}</span>
            </div>
            <div v-if="room?.candidate?.email" class="flex justify-between gap-3">
              <span class="shrink-0 text-muted-foreground">邮箱</span>
              <span class="min-w-0 break-all text-right tabular-nums">{{ room.candidate.email }}</span>
            </div>
          </div>
        </CardContent>
      </Card>

      <!-- 个人简介：长文本单独成卡 -->
      <Card v-if="room?.candidate?.profile">
        <CardContent class="space-y-2 p-4">
          <p class="text-sm font-medium">个人简介</p>
          <p class="whitespace-pre-line break-words text-sm">{{ room.candidate.profile }}</p>
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
              v-for="(c, i) in pullPool"
              :key="c.id"
              class="flex items-center justify-between gap-2 rounded-md bg-muted p-2"
            >
              <!-- 序号 = 候场队列名次（与候场大屏同口径：手动优先级优先，缺省按签到先后） -->
              <span
                class="inline-flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold tabular-nums text-primary"
                :title="`候队列第 ${i + 1} 位（先到在先；管理员可在候场大屏手动调序）`"
              >
                {{ i + 1 }}
              </span>
              <span class="min-w-0 flex-1 truncate text-sm">
                {{ c.name }}
                <span v-if="c.profile" class="text-xs text-muted-foreground">· {{ c.profile }}</span>
              </span>
              <Button size="sm" variant="outline" @click="emit('pull', c.id)">拉取</Button>
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
              class="flex min-w-0 gap-3"
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
                class="min-w-0 flex-1 break-words pb-4 text-xs leading-none"
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
            @click="emit('advance')"
          >
            推进到「{{ nextPhase ? STATUS_PRESENTATION[nextPhase].label : '—' }}」
          </Button>
        </CardContent>
      </Card>
    </div>

    <!-- 志愿与调剂编辑（独立小权限）；保存后父级重拉房间快照 -->
    <CandidatePreferenceDialog
      :open="!!preferenceTarget"
      :candidate="preferenceTarget"
      @update:open="preferenceTarget = $event ? preferenceTarget : null"
      @saved="emit('saved')"
    />
  </aside>
</template>
