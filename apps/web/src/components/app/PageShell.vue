<!--
  PageShell —— 管理页统一外壳：全高滚动容器 + 居中内容列（max-w-5xl）+ 页头。
  收编原先 4 个视图各自手写、且间距曾漂移（py-6/py-8、space-y-4/space-y-6）的实现。
  页头 = 标题 + 描述 + 右侧动作插槽；无标题时仅输出内容列。
-->
<script setup lang="ts">
const props = defineProps<{
  title?: string
  description?: string
}>()
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="mx-auto max-w-5xl space-y-4 px-4 py-6">
      <header v-if="props.title" class="flex items-end justify-between gap-3">
        <div class="min-w-0">
          <h1 class="text-2xl font-semibold tracking-tight">{{ props.title }}</h1>
          <p v-if="props.description" class="mt-1 text-sm text-muted-foreground">
            {{ props.description }}
          </p>
        </div>
        <!-- 页头右侧动作区（刷新 / 新增等）。 -->
        <div v-if="$slots.actions" class="flex shrink-0 items-center gap-2">
          <slot name="actions" />
        </div>
      </header>

      <slot />
    </div>
  </div>
</template>
