<!--
  MessageTranscript —— 只读消息回放列表（气泡样式与房间实时聊天一致）。
  房间外的归档场景（候选人详情侧栏、面试记录整页）共用，保证同一视觉语言。
-->
<script setup lang="ts">
import { senderLabel } from '@/domain/messages'
import { formatDateTime } from '@/lib/format'
import type { Message } from '@/models'

import { chatBubbleVariants } from '@/components/ui/tokens'
import { useAuth } from '@/composables/useAuth'
import { cn } from '@/lib/utils'

defineProps<{
  /** 按时间升序的消息列表。 */
  messages: Message[]
}>()

const { currentUserId } = useAuth()

function labelOf(m: Message, currentUserId: number | null): string {
  return senderLabel(m.sender_id, currentUserId, m.sender?.name || m.sender?.username)
}
</script>

<template>
  <div role="log" aria-label="面试记录" class="space-y-3">
    <div
      v-for="m in messages"
      :key="m.id"
      class="flex max-w-full flex-col gap-1"
      :class="m.sender_id === currentUserId ? 'items-end' : 'items-start'"
    >
      <div class="flex items-baseline gap-2 px-1 text-xs text-muted-foreground">
        <span class="font-medium">{{ labelOf(m, currentUserId) }}</span>
        <time>{{ formatDateTime(m.created_at) }}</time>
      </div>
      <div :class="cn(chatBubbleVariants({ side: m.sender_id === currentUserId ? 'own' : 'other' }))">
        {{ m.content }}
      </div>
    </div>
  </div>
</template>
