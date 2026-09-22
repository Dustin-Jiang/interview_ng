<!--
  ThemeToggle —— 深色模式切换（浅色 / 深色 / 跟随系统）。
  触发按钮图标跟随**实际生效**主题（跟随系统时也能一眼看出当前是深是浅）；
  三态在 Popover 里列出，当前模式打勾（用 aria-pressed 表达选中，不引入伪造的 menu 角色）。
  顶栏与登录页共用：登录前也要能选主题，否则深色用户被系统浅色偏好锁住。
-->
<script setup lang="ts">
import { computed, ref, type Component } from 'vue'
import { Check, Monitor, Moon, Sun } from 'lucide-vue-next'

import { useTheme } from '@/composables/useTheme'
import type { ThemeMode } from '@/domain/theme'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

const { mode, resolved, setMode } = useTheme()

const open = ref(false)

/** 三档模式（数组顺序即展示顺序）。 */
const OPTIONS: readonly { mode: ThemeMode; label: string; icon: Component }[] = [
  { mode: 'light', label: '浅色', icon: Sun },
  { mode: 'dark', label: '深色', icon: Moon },
  { mode: 'system', label: '跟随系统', icon: Monitor },
]

const currentLabel = computed(() => OPTIONS.find((o) => o.mode === mode.value)?.label ?? '')
const triggerIcon = computed(() => (resolved.value === 'dark' ? Moon : Sun))

function select(next: ThemeMode): void {
  setMode(next)
  open.value = false
}
</script>

<template>
  <Popover :open="open" @update:open="open = $event">
    <PopoverTrigger as-child>
      <Button variant="ghost" size="icon" :aria-label="`切换主题（当前：${currentLabel}）`">
        <component :is="triggerIcon" aria-hidden="true" />
      </Button>
    </PopoverTrigger>
    <PopoverContent align="end" class="w-40 p-1">
      <Button
        v-for="opt in OPTIONS"
        :key="opt.mode"
        variant="ghost"
        size="sm"
        class="w-full justify-start gap-2"
        :aria-pressed="mode === opt.mode"
        @click="select(opt.mode)"
      >
        <component :is="opt.icon" aria-hidden="true" />
        {{ opt.label }}
        <Check v-if="mode === opt.mode" class="ml-auto" aria-hidden="true" />
      </Button>
    </PopoverContent>
  </Popover>
</template>
