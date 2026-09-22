<!--
  MasterDetailSplit —— 名册 ↔ 详情两段式布局外壳。
  桌面端左右分栏（left aside + 右 main），lg 以下按 showDetail 在「列表 / 详情」间切换，
  详情态顶部提供返回列表按钮；详情内容固定在 max-w-3xl 的居中滚动列内。
  lg 以下单栏模式：aside 显式 w-full（列表/详情两态各自占满屏宽），否则 flex 主轴上的
  auto 宽度会被名册里的超宽内容（长房间名等）撑到整页横向滚动。
-->
<script setup lang="ts">
import { computed } from 'vue'
import { ArrowLeft } from 'lucide-vue-next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

const props = withDefaults(
  defineProps<{
    /** 移动端是否处于详情态（桌面端恒为分栏，该值不影响 lg 以上布局）。 */
    showDetail: boolean
    /** 两侧区域的 aria-label。 */
    asideLabel: string
    detailLabel: string
    /** 移动端返回按钮的无障碍标签。 */
    backAriaLabel: string
    /** 名册栏宽度档（lg 断点以上）。 */
    asideWidth?: string
  }>(),
  { asideWidth: 'lg:w-80' },
)

defineEmits<{ back: [] }>()

const asideClass = computed(() => (props.showDetail ? 'hidden lg:flex' : 'flex'))
const detailClass = computed(() => (props.showDetail ? 'flex' : 'hidden lg:flex'))
</script>

<template>
  <!-- 整体居中并限制最大宽度（max-w-content 语义档位），左右分栏 -->
  <div class="mx-auto flex h-full w-full max-w-content overflow-hidden">
    <aside
      :class="cn(asideClass, 'h-full min-w-0 shrink-0 flex-col max-lg:w-full', props.asideWidth)"
      :aria-label="props.asideLabel"
    >
      <slot name="aside" />
    </aside>

    <section
      :class="cn(detailClass, 'h-full min-w-0 max-lg:w-full flex-1 flex-col')"
      :aria-label="props.detailLabel"
    >
      <!-- 移动端返回栏 -->
      <div class="flex shrink-0 items-center gap-2 px-4 py-2 lg:hidden">
        <Button variant="ghost" size="sm" :aria-label="props.backAriaLabel" @click="$emit('back')">
          <ArrowLeft aria-hidden="true" />
          返回列表
        </Button>
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto">
        <div class="mx-auto max-w-3xl space-y-6 px-4 py-6">
          <slot name="detail" />
        </div>
      </div>
    </section>
  </div>
</template>
