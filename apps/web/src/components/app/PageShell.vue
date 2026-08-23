<!--
  PageShell —— 管理页统一外壳：全高滚动容器 + 居中内容列（max-w-content）+ 页头。
  页头 = 标题 + 右侧动作插槽；无标题时仅输出内容列。
-->
<script setup lang="ts">
const props = defineProps<{
  title?: string
}>()
</script>

<template>
  <div class="h-full overflow-y-auto">
    <div class="mx-auto max-w-content space-y-4 px-4 py-6">
      <header v-if="props.title" class="flex items-end justify-between gap-3">
        <h1 class="text-2xl font-semibold tracking-tight">{{ props.title }}</h1>
        <!-- 页头右侧动作区（刷新 / 新增等）。 -->
        <div v-if="$slots.actions" class="flex shrink-0 items-center gap-2">
          <slot name="actions" />
        </div>
      </header>

      <slot />
    </div>
  </div>
</template>
