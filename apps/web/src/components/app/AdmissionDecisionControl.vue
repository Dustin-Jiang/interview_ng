<!--
  AdmissionDecisionControl —— 本部门对当前候选人的录取表态（待定 / 录取 / 放弃 三档段式控件）。
  **只有这一处实现**：候选人查看页与面试房间左栏都渲染它，选项、标签、当前档高亮因此不会各自漂移。
  纯展示 + 向上抛事件：状态由调用方的 `useAdmissions` 持有（决定列表来自共享数据源 `useAdmissionList`），
  本组件只负责「画出三档 + 把点击转成 select」。
  key-hints：项内的 1/2/3 键帽**只在该页真的注册了这三个键时**开启（目前只有候选人查看页，
  由 `useRosterHotkeys` 的 `onOtherKey` 处理）——否则等于提示一个按了没反应的键。
-->
<script setup lang="ts">
import { ADMISSION_STATUSES, type AdmissionStatus } from '@/models'
import { ADMISSION_PRESENTATION } from '@/presenters/status'
import { Kbd } from '@/components/ui/kbd'
import { segmentedItemVariants } from '@/components/ui/tokens'

const props = withDefaults(
  defineProps<{
    /** 本部门已记录的决定；未记录（未表态）传 undefined。 */
    value?: AdmissionStatus
    /** 加载中/不可写：整组禁用。 */
    disabled?: boolean
    /** 项内给出 1/2/3 键帽（仅在该页注册了这三个快捷键时开启）。 */
    keyHints?: boolean
  }>(),
  { value: undefined, disabled: false, keyHints: false },
)

const emit = defineEmits<{ select: [status: AdmissionStatus] }>()
</script>

<template>
  <div
    class="inline-flex items-center rounded-lg bg-muted p-1"
    role="group"
    :aria-label="props.keyHints ? '本部门录取决定（快捷键 1/2/3）' : '本部门录取决定'"
  >
    <!-- 项内键帽 = 该档的快捷键序号（下标即 1/2/3，与各页取档的同一口径）；
         轨道是 bg-muted，故键帽用 surface 底色，否则会与轨道同色看不见。 -->
    <button
      v-for="(s, i) in ADMISSION_STATUSES"
      :key="s"
      type="button"
      :class="segmentedItemVariants({ active: props.value === s })"
      :disabled="props.disabled"
      @click="emit('select', s)"
    >
      {{ ADMISSION_PRESENTATION[s].label }}
      <Kbd v-if="props.keyHints" tone="surface">{{ i + 1 }}</Kbd>
    </button>
  </div>
</template>
