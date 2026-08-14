<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { toast } from 'vue-sonner'
import { Plus, RefreshCw } from 'lucide-vue-next'

import { useCandidates } from '@/composables/useCandidates'
import { CANDIDATE_STATUSES, type Candidate } from '@/models'
import { STATUS_PRESENTATION } from '@/presenters/status'
import { formatDateTime } from '@/lib/format'

// --- shadcn-vue UI ---
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'

const router = useRouter()
// 组合式函数（函数式 ViewModel）：顶层解构，模板直接引用（ref 自动解包）。
const { candidates, statusFilter, loading, load, create, checkin, assign, setStatusFilter } = useCandidates()
onMounted(() => load())

// 创建候选人对话框状态
const createOpen = ref(false)
const newName = ref('')
const newProfile = ref('')

// 分配对话框状态
const assignOpen = ref(false)
const assignTarget = ref<Candidate | null>(null)
const assignRoomId = ref('')

function openCreate() {
  newName.value = ''
  newProfile.value = ''
  createOpen.value = true
}

async function submitCreate() {
  if (!newName.value.trim()) {
    toast.error('请输入候选人姓名')
    return
  }
  try {
    await create(newName.value.trim(), newProfile.value.trim())
    createOpen.value = false
    toast.success('候选人已创建')
  } catch (e) {
    toast.error((e as Error).message)
  }
}

function openAssign(c: Candidate) {
  assignTarget.value = c
  assignRoomId.value = c.room_id ? String(c.room_id) : ''
  assignOpen.value = true
}

async function submitAssign() {
  if (!assignTarget.value) return
  const roomId = assignRoomId.value.trim() ? Number(assignRoomId.value.trim()) : undefined
  try {
    const assigned = await assign(assignTarget.value.id, roomId)
    toast.success(`已分配到房间 ${assigned}`)
    assignOpen.value = false
  } catch (e) {
    toast.error((e as Error).message)
  }
}

async function handleCheckin(c: Candidate) {
  try {
    await checkin(c.id)
    toast.success('签到成功')
  } catch (e) {
    toast.error((e as Error).message)
  }
}

function goRoom(roomId?: number) {
  if (!roomId) {
    toast.error('该候选人尚未分配房间')
    return
  }
  router.push({ name: 'room', params: { roomId: String(roomId) } })
}

onMounted(() => load())
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="mx-auto max-w-6xl space-y-4 px-4 py-6">
    <div class="flex items-end justify-between">
      <div>
        <h1 class="text-2xl font-semibold tracking-tight">候选人管理</h1>
        <p class="text-sm text-muted-foreground">浏览、创建、签到并在面试房间中分配候选人。</p>
      </div>
      <div class="flex items-center gap-2">
        <Button variant="outline" size="icon" aria-label="刷新" @click="load">
          <RefreshCw class="h-4 w-4" />
        </Button>
        <Dialog :open="createOpen" @update:open="createOpen = $event">
          <DialogTrigger as-child>
            <Button @click="openCreate">
              <Plus class="h-4 w-4" />
              新增候选人
            </Button>
          </DialogTrigger>
          <DialogContent class="sm:max-w-[425px]">
            <DialogHeader>
              <DialogTitle>新增候选人</DialogTitle>
              <DialogDescription>填写候选人信息后加入面试队列。</DialogDescription>
            </DialogHeader>
            <div class="grid gap-4">
              <div class="grid gap-2">
                <Label for="cand-name">姓名</Label>
                <Input id="cand-name" v-model="newName" placeholder="候选人姓名" />
              </div>
              <div class="grid gap-2">
                <Label for="cand-profile">个人简介</Label>
                <Input id="cand-profile" v-model="newProfile" placeholder="技术栈 / 背景（可选）" />
              </div>
            </div>
            <DialogFooter>
              <Button variant="outline" @click="createOpen = false">取消</Button>
              <Button @click="submitCreate">创建</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    </div>

    <Card>
      <CardHeader class="flex-row items-center justify-between gap-4 space-y-0">
        <div>
          <CardTitle>候选人列表</CardTitle>
          <CardDescription>按状态筛选，可签到 / 分配 / 查看。</CardDescription>
        </div>
        <Select :model-value="statusFilter || 'ALL'" @update:model-value="setStatusFilter($event === 'ALL' ? '' : ($event as any))">
          <SelectTrigger class="w-[180px]">
            <SelectValue placeholder="全部状态" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">全部状态</SelectItem>
            <SelectItem v-for="s in CANDIDATE_STATUSES" :key="s" :value="s">
              {{ STATUS_PRESENTATION[s].label }}
            </SelectItem>
          </SelectContent>
        </Select>
      </CardHeader>
      <CardContent>
        <!-- 加载态 -->
        <div v-if="loading" class="space-y-2 py-4">
          <Skeleton v-for="i in 4" :key="i" class="h-10 w-full" />
        </div>

        <!-- 空态 -->
        <div v-else-if="candidates.length === 0" class="py-12 text-center text-muted-foreground">
          暂无候选人
        </div>

        <!-- 数据表格 -->
        <Table v-else>
          <TableHeader>
            <TableRow>
              <TableHead class="w-[60px]">ID</TableHead>
              <TableHead>姓名</TableHead>
              <TableHead>状态</TableHead>
              <TableHead>房间</TableHead>
              <TableHead>简介</TableHead>
              <TableHead>创建时间</TableHead>
              <TableHead class="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="c in candidates" :key="c.id">
              <TableCell class="font-mono text-xs">{{ c.id }}</TableCell>
              <TableCell class="font-medium">{{ c.name }}</TableCell>
              <TableCell>
                <Badge :variant="STATUS_PRESENTATION[c.status].badge">
                  {{ STATUS_PRESENTATION[c.status].label }}
                </Badge>
              </TableCell>
              <TableCell>
                <Button v-if="c.room_id" variant="link" class="h-auto p-0" @click="goRoom(c.room_id)">
                  #{{ c.room_id }}
                </Button>
                <span v-else class="text-muted-foreground">-</span>
              </TableCell>
              <TableCell class="max-w-[220px] truncate text-muted-foreground">{{ c.profile || '-' }}</TableCell>
              <TableCell class="text-muted-foreground">{{ formatDateTime(c.created_at) }}</TableCell>
              <TableCell class="text-right">
                <div class="flex justify-end gap-2">
                  <Button
                    v-if="c.status === 'NOT_CHECKED_IN'"
                    size="sm"
                    variant="secondary"
                    @click="handleCheckin(c)"
                  >
                    签到
                  </Button>
                  <Button
                    v-if="['NOT_CHECKED_IN', 'CHECKED_IN_PENDING_ASSIGN'].includes(c.status)"
                    size="sm"
                    @click="openAssign(c)"
                  >
                    分配
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>

    <!-- 分配对话框 -->
    <Dialog :open="assignOpen" @update:open="assignOpen = $event">
      <DialogContent class="sm:max-w-[380px]">
        <DialogHeader>
          <DialogTitle>分配候选人</DialogTitle>
          <DialogDescription>
            为「{{ assignTarget?.name }}」分配面试房间，留空则自动新建。
          </DialogDescription>
        </DialogHeader>
        <div class="grid gap-2">
          <Label for="assign-room">房间 ID（可选）</Label>
          <Input id="assign-room" v-model="assignRoomId" inputmode="numeric" placeholder="留空自动新建" />
        </div>
        <DialogFooter>
          <Button variant="outline" @click="assignOpen = false">取消</Button>
          <Button @click="submitAssign">分配</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
    </div>
  </div>
</template>
