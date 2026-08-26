<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { DoorOpen, Plus, RefreshCw, Trash2 } from 'lucide-vue-next'

import { useRoomList } from '@/composables/useRoomList'
import { useAuth } from '@/composables/useAuth'
import { useBoardRefresh } from '@/composables/useBoardChannel'
import { roomApi } from '@/api/http'
import { PERMISSIONS, type CandidateStatus } from '@/models'
import { roomPhaseOf } from '@/domain/status'
import { STATUS_PRESENTATION, EMPTY_PRESENTATION } from '@/presenters/status'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { tileVariants } from '@/components/ui/tokens'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import EmptyState from '@/components/app/EmptyState.vue'
import PageShell from '@/components/app/PageShell.vue'

import RoomChat from './RoomChat.vue'

const route = useRoute()
const router = useRouter()
const { hasPermission } = useAuth()
// 组合式函数（函数式 ViewModel）：房间列表。
const { rooms, loading, load } = useRoomList()

const parsedRoomId = computed<number | null>(() => {
  const raw = route.params.roomId ? String(route.params.roomId) : ''
  const n = Number(raw)
  return Number.isInteger(n) && n > 0 ? n : null
})

onMounted(() => {
  // 列表态才拉取房间列表；房间态由 RoomChat 内部自行连接。
  if (!parsedRoomId.value) load()
})

// 从房间返回列表时重新加载房间列表。
watch(parsedRoomId, (id) => {
  if (!id) load()
})

// ---- 实时刷新：房间/候选人状态变化（建删房、拉取、阶段、清房）→ 防抖重拉列表 ----
// 仅列表态生效；进入具体房间后由 RoomChat 的房间通道负责实时。
useBoardRefresh(
  [
    'room_created',
    'room_deleted',
    'candidate_assigned',
    'candidate_deleted',
    'room_phase_changed',
  ],
  () => {
    if (!parsedRoomId.value) void load()
  },
)

function openRoom(id: number) {
  router.push({ name: 'room', params: { roomId: String(id) } })
}

function backToList() {
  router.push({ name: 'room' })
}

function statusOf(room: import('@/models').Room): { label: string; badge: 'outline' | 'secondary' | 'default' | 'destructive' } {
  const s = roomPhaseOf(room) as CandidateStatus | null
  return s ? STATUS_PRESENTATION[s] : EMPTY_PRESENTATION
}

async function createRoom() {
  try {
    const res = await roomApi.create()
    toast.success(`已创建房间 #${res.id}`)
    await load()
  } catch (e) {
    toast.error((e as Error).message)
  }
}

// 删除空房确认对话框（替代 window.confirm）。
const deleteTarget = ref<import('@/models').Room | null>(null)
const deleting = ref(false)

async function confirmDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    await roomApi.remove(deleteTarget.value.id)
    toast.success(`已删除房间 #${deleteTarget.value.id}`)
    deleteTarget.value = null
    await load()
  } catch (e) {
    toast.error((e as Error).message)
  } finally {
    deleting.value = false
  }
}
</script>

<template>
  <!-- 已进入具体房间：渲染聊天界面 -->
  <RoomChat
    v-if="parsedRoomId"
    :key="parsedRoomId"
    :room-id="parsedRoomId"
    @back="backToList"
  />

  <!-- 房间列表 -->
  <PageShell v-else title="面试房间">
    <template #actions>
      <Button
        v-if="hasPermission(PERMISSIONS.ROOMS_MANAGE)"
        size="sm"
        @click="createRoom"
      >
        <Plus aria-hidden="true" />
        新建房间
      </Button>
      <Button variant="outline" size="icon" aria-label="刷新房间列表" @click="load">
        <RefreshCw :class="loading ? 'animate-spin' : ''" aria-hidden="true" />
      </Button>
    </template>

    <!-- 空态：说明 + 引导动作 -->
    <EmptyState v-if="!loading && rooms.length === 0" :icon="DoorOpen">
      暂无面试房间
      <template v-if="hasPermission(PERMISSIONS.ROOMS_MANAGE)" #action>
        <Button size="sm" variant="outline" @click="createRoom">
          <Plus aria-hidden="true" />
          新建第一个房间
        </Button>
      </template>
    </EmptyState>

    <!-- 加载骨架屏 -->
    <div v-else-if="loading && rooms.length === 0" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3" aria-busy="true">
      <Skeleton v-for="i in 6" :key="i" class="h-[104px] rounded-xl" />
    </div>

    <!-- 房间卡片网格（tileVariants 统一可交互表面） -->
    <div v-else class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <div
        v-for="room in rooms"
        :key="room.id"
        :class="cn(tileVariants(), 'group flex flex-col overflow-hidden')"
      >
        <!-- 可点击主区：进入房间（独立按钮，避免与操作区嵌套） -->
        <button
          class="flex min-w-0 flex-1 cursor-pointer p-4 text-left outline-none"
          :aria-label="`进入房间 #${room.id}`"
          @click="openRoom(room.id)"
        >
          <span class="flex w-full items-start justify-between gap-2">
            <span class="min-w-0">
              <span class="block truncate font-medium">房间 #{{ room.id }}</span>
              <span class="mt-0.5 block truncate text-sm text-muted-foreground">
                {{ room.candidate?.name ?? '空闲' }}
              </span>
            </span>
            <Badge :variant="statusOf(room).badge" class="shrink-0">
              {{ statusOf(room).label }}
            </Badge>
          </span>
        </button>
        <!-- 卡片脚注：时间与操作分离，不再重叠 -->
        <div class="flex items-center justify-between border-t bg-muted/30 px-4 py-2">
          <time class="text-xs text-muted-foreground">{{ formatDateTime(room.created_at) }}</time>
          <Button
            v-if="hasPermission(PERMISSIONS.ROOMS_MANAGE) && !room.candidate"
            variant="ghost"
            size="sm"
            class="h-7 gap-1 px-2 text-xs text-destructive hover:text-destructive"
            @click="deleteTarget = room"
          >
            <Trash2 aria-hidden="true" />
            删除空房
          </Button>
        </div>
      </div>
    </div>

    <!-- 删除空房确认 -->
    <ConfirmDialog
      :open="!!deleteTarget"
      title="删除空房间"
      confirm-text="删除"
      destructive
      :loading="deleting"
      @update:open="deleteTarget = $event ? deleteTarget : null"
      @confirm="confirmDelete"
    />
  </PageShell>
</template>
