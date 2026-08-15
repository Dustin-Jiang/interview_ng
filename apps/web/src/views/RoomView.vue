<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { DoorOpen, Plus, RefreshCw } from 'lucide-vue-next'

import { useRoomList, roomStatus } from '@/composables/useRoomList'
import { useAuth } from '@/composables/useAuth'
import { roomApi } from '@/api/http'
import { PERMISSIONS, type CandidateStatus } from '@/models'
import { STATUS_PRESENTATION, EMPTY_PRESENTATION } from '@/presenters/status'
import { formatDateTime } from '@/lib/format'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

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

function openRoom(id: number) {
  router.push({ name: 'room', params: { roomId: String(id) } })
}

function backToList() {
  router.push({ name: 'room' })
}

function statusOf(room: import('@/models').Room): { label: string; badge: 'outline' | 'secondary' | 'default' | 'destructive' } {
  const s = roomStatus(room) as CandidateStatus | null
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

async function deleteEmptyRoom(roomId: number) {
  if (!window.confirm(`确认删除空房间 #${roomId}？`)) return
  try {
    await roomApi.remove(roomId)
    toast.success(`已删除房间 #${roomId}`)
    await load()
  } catch (e) {
    toast.error((e as Error).message)
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
  <div v-else class="h-full overflow-y-auto">
    <div class="mx-auto max-w-6xl space-y-4 px-4 py-6">
      <div class="flex items-end justify-between">
        <div>
          <h1 class="text-2xl font-semibold tracking-tight">面试房间</h1>
        </div>
        <div class="flex items-center gap-2">
          <Button
            v-if="hasPermission(PERMISSIONS.ROOMS_MANAGE)"
            size="sm"
            @click="createRoom"
          >
            <Plus class="h-4 w-4" />
            新建房间
          </Button>
          <Button variant="outline" size="icon" aria-label="刷新" @click="load">
            <RefreshCw class="h-4 w-4" />
          </Button>
        </div>
      </div>

      <!-- 加载态 -->
      <div v-if="loading" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <Skeleton v-for="i in 6" :key="i" class="h-24 w-full" />
      </div>

      <!-- 空态 -->
      <Card v-else-if="rooms.length === 0">
        <CardContent class="flex flex-col items-center justify-center gap-3 py-14 text-center text-muted-foreground">
          <DoorOpen class="h-8 w-8" />
          <p>暂无面试房间</p>
        </CardContent>
      </Card>

      <!-- 房间卡片网格 -->
      <div v-else class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <div
          v-for="room in rooms"
          :key="room.id"
          class="group relative rounded-xl border bg-card text-left shadow-sm transition-colors hover:bg-accent/40 hover:shadow"
        >
          <button class="w-full p-0 text-left" @click="openRoom(room.id)">
            <CardContent class="p-4">
              <div class="flex items-start justify-between gap-2">
                <div class="min-w-0">
                  <p class="truncate font-medium">
                    房间 #{{ room.id }}
                  </p>
                  <p class="mt-0.5 truncate text-sm text-muted-foreground">
                    {{ room.candidate?.name ?? '空闲' }}
                  </p>
                </div>
                <Badge :variant="statusOf(room).badge">
                  {{ statusOf(room).label }}
                </Badge>
              </div>
              <p class="mt-3 text-xs text-muted-foreground">
                {{ formatDateTime(room.created_at) }}
              </p>
            </CardContent>
          </button>
          <Button
            v-if="hasPermission(PERMISSIONS.ROOMS_MANAGE) && !room.candidate"
            variant="ghost"
            size="sm"
            class="absolute right-2 top-10 text-destructive"
            @click="deleteEmptyRoom(room.id)"
          >
            删除空房
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>
