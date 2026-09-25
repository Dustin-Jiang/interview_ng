<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { DoorOpen, Pencil, Plus, Trash2 } from 'lucide-vue-next'

import { useRooms } from '@/composables/useRooms'
import { useAuth } from '@/composables/useAuth'
import { useBoardRefresh } from '@/composables/useBoardChannel'
import { useConfirmAction } from '@/composables/useConfirmAction'
import { useEntityDialog } from '@/composables/useEntityDialog'
import { roomApi } from '@/api/http'
import { PERMISSIONS, type CandidateStatus, type Room } from '@/models'
import { roomPhaseOf } from '@/domain/status'
import { roomLabel } from '@/domain/room'
import { STATUS_PRESENTATION, EMPTY_PRESENTATION } from '@/presenters/status'
import { formatDateTime } from '@/lib/format'
import { cn } from '@/lib/utils'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { tileVariants } from '@/components/ui/tokens'
import ConfirmDialog from '@/components/app/ConfirmDialog.vue'
import EmptyState from '@/components/app/EmptyState.vue'
import FormDialog from '@/components/app/FormDialog.vue'
import ListSkeleton from '@/components/app/ListSkeleton.vue'
import PageShell from '@/components/app/PageShell.vue'
import RefreshButton from '@/components/app/RefreshButton.vue'

import RoomChat from './RoomChat.vue'

const route = useRoute()
const router = useRouter()
const { hasPermission } = useAuth()
// 房间列表（共享数据源，`GET /rooms` 全站唯一一份）：列表页要全量，按 id 反查名字的三处要映射，
// 都读它。房间**建/删/改名**的刷新由该数据源统一订阅（见 domain/room.ts#ROOM_BOARD_EVENTS），
// 本页只额外订阅「房间卡片上显示的内容」那些变化（下方 useBoardRefresh）。
const { rooms, loading, load } = useRooms()

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

// ---- 实时刷新：房间卡片上「当下是谁、进行到哪一档」的变化 → 防抖重拉列表 ----
// 仅列表态生效；房间**自身**的建/删/改名已由 useRooms 的数据源订阅覆盖，这里不重复订阅。
// 进入具体房间后由 RoomChat 的房间通道负责实时。
useBoardRefresh(
  ['candidate_assigned', 'candidate_deleted', 'room_phase_changed'],
  () => {
    if (!parsedRoomId.value) void load()
  },
)

function openRoom(id: number) {
  router.push({ name: 'room', params: { roomId: String(id) } })
}

function backToList() {
  router.push({ name: 'rooms' })
}

function statusOf(room: Room): { label: string; badge: 'outline' | 'secondary' | 'default' | 'destructive' } {
  const s = roomPhaseOf(room) as CandidateStatus | null
  return s ? STATUS_PRESENTATION[s] : EMPTY_PRESENTATION
}

/** 房间显示名：统一走 domain/room（未命名 → 「未命名」，不显示编号）。 */

// ---- 建房 / 改名对话框（同一表单两态：target 为空即新建） ----
const {
  open: nameDialogOpen,
  editing: renaming,
  form: nameForm,
  saving: savingName,
  openCreate,
  openEdit: renameRoom,
  submit: submitName,
} = useEntityDialog<Room, string>({
  blank: () => '',
  toForm: (room) => room.name ?? '',
  action: async (name, room) => {
    const trimmed = name.trim()
    if (room) await roomApi.rename(room.id, trimmed)
    else await roomApi.create(trimmed)
    await load()
  },
  success: (name, room) => {
    const trimmed = name.trim()
    if (room) return trimmed ? `已改名为「${trimmed}」` : '已清除命名'
    return trimmed ? `已创建房间「${trimmed}」` : '已创建未命名房间'
  },
})

// 删除空房确认对话框（替代 window.confirm）。
const {
  target: deleteTarget,
  loading: deleting,
  request: requestDelete,
  onOpenChange: onDeleteOpenChange,
  confirm: confirmDelete,
} = useConfirmAction<Room>({
  action: async (room) => {
    await roomApi.remove(room.id)
    await load()
  },
  success: (room) => `已删除${roomLabel(room)}`,
})
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
        @click="openCreate"
      >
        <Plus aria-hidden="true" />
        新建房间
      </Button>
      <RefreshButton label="刷新房间列表" :loading="loading" @click="load" />
    </template>

    <!-- 空态：说明 + 引导动作 -->
    <EmptyState v-if="!loading && rooms.length === 0" :icon="DoorOpen">
      暂无面试房间
      <template v-if="hasPermission(PERMISSIONS.ROOMS_MANAGE)" #action>
        <Button size="sm" variant="outline" @click="openCreate">
          <Plus aria-hidden="true" />
          新建第一个房间
        </Button>
      </template>
    </EmptyState>

    <!-- 加载骨架屏 -->
    <ListSkeleton
      v-else-if="loading && rooms.length === 0"
      layout="grid"
      :rows="6"
      item-class="h-[104px] rounded-xl"
    />

    <!-- 房间卡片网格（tileVariants 统一可交互表面）：手机单列，md 起两列，lg 三列 -->
    <div v-else class="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
      <div
        v-for="room in rooms"
        :key="room.id"
        :class="cn(tileVariants(), 'group flex flex-col overflow-hidden')"
      >
        <!-- 可点击主区：进入房间（独立按钮，避免与操作区嵌套） -->
        <button
          class="flex min-w-0 flex-1 cursor-pointer p-4 text-left outline-none"
          :aria-label="`进入${roomLabel(room)}`"
          @click="openRoom(room.id)"
        >
          <span class="flex w-full items-start justify-between gap-2">
            <span class="min-w-0">
              <span class="block truncate font-medium">{{ roomLabel(room) }}</span>
              <span class="mt-0.5 block truncate text-sm text-muted-foreground">
                {{ room.candidate?.name ?? '空闲' }}
              </span>
            </span>
            <Badge :variant="statusOf(room).badge" class="shrink-0">
              {{ statusOf(room).label }}
            </Badge>
          </span>
        </button>
        <!-- 卡片脚注：时间与操作分离，不再重叠；窄屏放不下时操作换行而非横向溢出 -->
        <div class="flex flex-wrap items-center justify-between gap-2 border-t bg-muted/30 px-4 py-2">
          <time class="text-xs text-muted-foreground">{{ formatDateTime(room.created_at) }}</time>
          <span class="flex flex-wrap items-center gap-1">
            <Button
              v-if="hasPermission(PERMISSIONS.ROOMS_MANAGE)"
              variant="ghost"
              size="sm"
              class="h-7 gap-1 px-2 text-xs text-muted-foreground hover:text-foreground max-lg:h-11 max-lg:text-sm"
              :aria-label="`改名：${roomLabel(room)}`"
              @click="renameRoom(room)"
            >
              <Pencil aria-hidden="true" />
              改名
            </Button>
            <Button
              v-if="hasPermission(PERMISSIONS.ROOMS_MANAGE) && !room.candidate"
              variant="ghost"
              size="sm"
              class="h-7 gap-1 px-2 text-xs text-destructive hover:text-destructive max-lg:h-11 max-lg:text-sm"
              @click="requestDelete(room)"
            >
              <Trash2 aria-hidden="true" />
              删除空房
            </Button>
          </span>
        </div>
      </div>
    </div>

    <!-- 建房 / 改名对话框 -->
    <FormDialog
      :open="nameDialogOpen"
      :title="renaming ? '房间改名' : '新建房间'"
      :submit-text="renaming ? '保存' : '创建'"
      size="sm"
      :loading="savingName"
      @update:open="nameDialogOpen = $event"
      @submit="submitName"
    >
      <div class="grid gap-2">
        <Label for="room-name">房间名</Label>
        <Input
          id="room-name"
          v-model="nameForm"
          :maxlength="64"
          @keydown.enter="submitName"
        />
      </div>
    </FormDialog>

    <!-- 删除空房确认 -->
    <ConfirmDialog
      :open="!!deleteTarget"
      title="删除空房间"
      confirm-text="删除"
      destructive
      :loading="deleting"
      @update:open="onDeleteOpenChange"
      @confirm="confirmDelete"
    />
  </PageShell>
</template>
