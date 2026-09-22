<!--
  CallNumberDialog —— 叫号弹窗（候场大屏专用）。
  候选人被拉取进房间（candidate_assigned）时弹出：大字报出姓名、学号与目标房间；
  大屏通常无人值守，倒计时结束自动关闭让位给下一位。
  受控组件：显示与否由父级队列决定，关闭一律经 dismiss 事件交回父级出队。
-->
<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { Megaphone } from 'lucide-vue-next'

import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogFooter, DialogTitle } from '@/components/ui/dialog'

/** 自动关闭秒数：够大屏读完一条，又不至于排队阻塞后续叫号。 */
const AUTO_DISMISS_SECONDS = 10

const props = defineProps<{
  /** 当前叫号；null = 不显示弹窗。 */
  notice: { name: string; studentNo: string; roomLabel: string } | null
  /** 队列中仍待叫号的人数。 */
  pending: number
}>()

const emit = defineEmits<{ dismiss: [] }>()

const remaining = ref(AUTO_DISMISS_SECONDS)
let timer: ReturnType<typeof setInterval> | undefined

function stopTimer(): void {
  if (timer === undefined) return
  clearInterval(timer)
  timer = undefined
}

// 换人（含出队后传下一位）即重置倒计时；notice 为 null 时停止计时。
watch(
  () => props.notice,
  (n) => {
    stopTimer()
    if (!n) return
    remaining.value = AUTO_DISMISS_SECONDS
    timer = setInterval(() => {
      remaining.value -= 1
      if (remaining.value > 0) return
      stopTimer()
      emit('dismiss')
    }, 1000)
  },
  { immediate: true },
)

onUnmounted(stopTimer)
</script>

<template>
  <Dialog :open="!!notice" @update:open="(v) => !v && emit('dismiss')">
    <!-- 小屏（360×640）压紧纵向留白与行距，保证大字报内容 + 主按钮一屏可见、无需滚动 -->
    <DialogContent
      size="lg"
      class="items-center gap-4 border-2 border-primary py-6 text-center sm:gap-5 sm:py-10"
    >
      <span
        class="flex h-20 w-20 animate-pulse items-center justify-center rounded-full bg-primary/10 text-primary"
        aria-hidden="true"
      >
        <Megaphone class="h-10 w-10" />
      </span>

      <DialogTitle class="text-xl font-semibold text-muted-foreground">请入场面试</DialogTitle>

      <p v-if="notice" class="break-words text-4xl font-black tracking-wide sm:text-5xl">{{ notice.name }}</p>
      <p v-if="notice" class="text-lg tabular-nums text-muted-foreground">学号 {{ notice.studentNo }}</p>
      <p v-if="notice" class="break-words text-2xl font-semibold text-primary sm:text-3xl">
        请前往 {{ notice.roomLabel }}
      </p>

      <DialogFooter class="flex-row flex-wrap items-center justify-center gap-3 sm:justify-center">
        <span v-if="pending > 0" class="text-sm tabular-nums text-muted-foreground">
          还有 {{ pending }} 位待叫号
        </span>
        <Button size="lg" @click="emit('dismiss')">知道了（{{ remaining }}s）</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>
</template>
