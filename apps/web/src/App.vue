<script setup lang="ts">
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { Toaster } from 'vue-sonner'
import { computed } from 'vue'

const route = useRoute()

const navItems = [
  { name: '候选人管理', to: { name: 'candidates' } },
  { name: '面试房间', to: { name: 'room' } },
]

/** 是否处于房间内部（全屏专注界面，header 由房间视图自绘，无全局导航）。 */
const isInRoom = computed(() => route.name === 'room' && !!route.params.roomId)
</script>

<template>
  <div class="flex h-screen flex-col overflow-hidden bg-background font-sans">
    <!-- 全局导航：仅在列表/管理场景显示 -->
    <header
      v-if="!isInRoom"
      class="z-40 w-full shrink-0 border-b bg-background/95 backdrop-blur"
    >
      <div class="mx-auto flex h-14 max-w-6xl items-center gap-6 px-4">
        <span class="text-lg font-semibold tracking-tight">面试系统 · 控制台</span>
        <nav class="flex items-center gap-1">
          <RouterLink
            v-for="item in navItems"
            :key="item.name"
            :to="item.to"
            class="rounded-md px-3 py-2 text-sm font-medium transition-colors"
            :class="
              route.name === item.to.name
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'
            "
          >
            {{ item.name }}
          </RouterLink>
        </nav>
      </div>
    </header>

    <!-- 主内容：全高 flex 布局，滚动在内部视图处理 -->
    <div class="min-h-0 flex-1">
      <RouterView />
    </div>

    <Toaster position="top-right" />
  </div>
</template>
