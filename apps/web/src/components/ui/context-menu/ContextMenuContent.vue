<script setup lang="ts">
import type { HTMLAttributes } from 'vue'
import type { ContextMenuContentEmits, ContextMenuContentProps } from 'reka-ui'
import { ContextMenuContent, ContextMenuPortal, useForwardPropsEmits } from 'reka-ui'
import { cn } from '@/lib/utils'

defineOptions({ inheritAttrs: false })

const props = defineProps<ContextMenuContentProps & { class?: HTMLAttributes['class'] }>()
const emits = defineEmits<ContextMenuContentEmits>()
const forwarded = useForwardPropsEmits(props, emits)
</script>

<template>
  <ContextMenuPortal>
    <ContextMenuContent
      v-bind="{ ...forwarded, ...$attrs }"
      :class="
        cn(
          // 弹层（消息右键菜单）的开关动效走动效令牌，按用途取值：
          //   开关不对等：开 --duration-fast(250ms)、关 --duration-quick(150ms)——关闭要更快让开视线；
          //   缩放：打开从 --scale-medium(0.97)、关闭到 --scale-tiny(0.99)，比 shadcn 默认的 0.95 克制
          //   （0.95 是弹窗量级，小弹层用它像「变焦」）；缓动统一 --ease-smooth-out（面板类位移默认档）。
          // 写法：本项目 tailwindcss-animate 的 `duration-[…]`/`ease-[…]` 任意值工具类不生成规则
          // （实测 0 条），必须写成任意属性 `[animation-duration:var(--…)]`。
        'z-50 min-w-[9rem] overflow-hidden rounded-md border bg-popover p-1 text-popover-foreground shadow-md outline-none [animation-timing-function:var(--ease-smooth-out)] data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-[0.99] data-[state=open]:zoom-in-[0.97] data-[state=closed]:[animation-duration:var(--duration-quick)] data-[state=open]:[animation-duration:var(--duration-fast)]',
          props.class,
        )
      "
    >
      <slot />
    </ContextMenuContent>
  </ContextMenuPortal>
</template>
