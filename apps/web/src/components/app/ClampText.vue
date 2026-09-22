<!--
  ClampText —— 多行截断文本：按行数 clamp（默认 3 行）+ 省略号；仅当内容真正溢出时
  可点击，用 Popover 显示全文（全文过长时 popover 内部滚动）。用于表格单元格等
  定宽窄容器承载长文本。
-->
<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch, type HTMLAttributes } from 'vue'

import { Popover, PopoverAnchor, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    /** 展示（与全文）文本。 */
    text: string
    /** 截断行数。 */
    lines?: number
    /** 文本容器附加类（宽度/颜色等由调用方控制）。 */
    class?: HTMLAttributes['class']
  }>(),
  { lines: 3 },
)

const el = ref<HTMLElement | null>(null)
/** 是否溢出（溢出才挂 popover，短文本不产生可点击的死区）。 */
const overflowing = ref(false)

function measure() {
  const node = el.value
  if (!node) return
  overflowing.value = node.scrollHeight - node.clientHeight > 1
}

onMounted(measure)
watch(() => [props.text, props.lines], () => void nextTick(measure))

/** 行数 → 截断类（Tailwind 只认静态类名，故枚举字面量）。 */
const CLAMP_CLASS: Record<number, string> = { 1: 'line-clamp-1', 2: 'line-clamp-2', 3: 'line-clamp-3', 4: 'line-clamp-4' }
const clampClass = computed(() => CLAMP_CLASS[props.lines] ?? CLAMP_CLASS[3])
</script>

<template>
  <Popover v-if="overflowing">
    <PopoverAnchor as-child>
      <div
        ref="el"
        :class="cn('relative whitespace-pre-line break-words', clampClass, props.class)"
      >
        {{ props.text }}
        <!-- 透明覆盖层即触发器：不新增可见元素、不占布局；聚焦时补焦点环。 -->
        <PopoverTrigger
          aria-label="查看全文"
          class="absolute inset-0 z-10 cursor-pointer rounded-sm bg-transparent outline-none focus-visible:ring-2 focus-visible:ring-ring"
        />
      </div>
    </PopoverAnchor>
    <PopoverContent align="start" class="max-h-72 w-[min(24rem,calc(100vw-2rem))] overflow-y-auto whitespace-pre-line break-words text-sm">
      {{ props.text }}
    </PopoverContent>
  </Popover>
  <div
    v-else
    ref="el"
    :class="cn('whitespace-pre-line break-words', clampClass, props.class)"
  >
    {{ props.text }}
  </div>
</template>
