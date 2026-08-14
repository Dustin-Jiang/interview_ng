<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { DoorOpen, RefreshCw } from 'lucide-vue-next'

import { useRoomList, roomStatus } from '@/composables/useRoomList'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { formatDateTime } from '@/lib/format'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

import RoomChat from './RoomChat.vue'

const route = useRoute()
const router = useRouter()
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
        <p class="text-sm text-muted-foreground">选择房间查看实时面试记录，点击进入。</p>
      </div>
      <Button variant="outline" size="icon" aria-label="刷新" @click="load">
        <RefreshCw class="h-4 w-4" />
      </Button>
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
        <p class="text-xs">在「候选人管理」中为候选人分配房间后即可在此查看。</p>
      </CardContent>
    </Card>

    <!-- 房间卡片网格 -->
    <div v-else class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <button
        v-for="room in rooms"
        :key="room.id"
        class="group rounded-xl border bg-card text-left shadow-sm transition-colors hover:bg-accent/40 hover:shadow"
        @click="openRoom(room.id)"
      >
        <CardContent class="p-4">
          <div class="flex items-start justify-between gap-2">
            <div class="min-w-0">
              <p class="truncate font-medium">
                房间 #{{ room.id }}
              </p>
              <p class="mt-0.5 truncate text-sm text-muted-foreground">
                {{ room.candidate?.name ?? '未绑定候选人' }}
              </p>
            </div>
            <Badge :variant="STATUS_PRESENTATION[roomStatus(room)].badge">
              {{ STATUS_PRESENTATION[roomStatus(room)].label }}
            </Badge>
          </div>
          <p class="mt-3 text-xs text-muted-foreground">
            {{ formatDateTime(room.created_at) }}
          </p>
        </CardContent>
      </button>
    </div>
    </div>
  </div>
</template>
