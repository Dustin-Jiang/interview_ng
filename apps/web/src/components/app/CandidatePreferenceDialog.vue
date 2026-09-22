<!--
  CandidatePreferenceDialog —— 志愿与调剂编辑弹窗（独立小权限 candidates.preferences）。
  三处入口共用：候选人详情 / 房间左栏 / 候选人管理表。
  志愿为**部门下拉**（取值来自 GET /api/departments，面试官只读该列表），另保留候选人现有取值
  作为额外选项（历史自由文本/已改名部门），避免保存时被静默清空。
  自持提交：PATCH /api/candidates/:id/preferences（只覆盖这三列，不动其他资料），
  成功后关闭并 emit saved，由父级按自身数据源刷新（列表重拉 / 房间重拉）。
-->
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toast } from 'vue-sonner'

import { departmentApi, candidateApi } from '@/api/http'
import type { Candidate, CandidatePreferencesPayload } from '@/models'
import { toastError } from '@/lib/toast'

import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import FormDialog from '@/components/app/FormDialog.vue'

const props = defineProps<{
  open: boolean
  /** 目标候选人；null = 无目标（弹窗不显示内容）。 */
  candidate: Candidate | null
}>()

const emit = defineEmits<{ 'update:open': [boolean]; saved: [] }>()

// 同页可能并存多个入口的实例，用自增后缀保证 label/input 的 id 唯一。
let seq = 0
const uid = `cand-pref-${++seq}`

const form = ref<CandidatePreferencesPayload>({
  first_choice: '',
  second_choice: '',
  accept_adjust: false,
})
const saving = ref(false)

/** 「无志愿」哨兵值：SelectItem 不接受空串值，提交时再转回空串。 */
const NO_CHOICE = '__none__'

/** 部门名单（志愿的取值来源，管理员维护）。 */
const departments = ref<string[]>([])

/** 可选志愿 = 部门名单 + 候选人现有取值（历史自由文本或已改名部门，避免保存时被静默清空）。 */
const choiceOptions = computed(() => {
  const names = departments.value
  const extra = [form.value.first_choice, form.value.second_choice].filter(
    (v) => v && !names.includes(v),
  )
  return [...names, ...extra]
})

/** 第一/第二志愿：'' ↔ 哨兵值互转，落库仍是「部门名 / 空串」。 */
const firstChoice = computed({
  get: () => form.value.first_choice || NO_CHOICE,
  set: (v: string) => {
    form.value.first_choice = v === NO_CHOICE ? '' : v
  },
})
const secondChoice = computed({
  get: () => form.value.second_choice || NO_CHOICE,
  set: (v: string) => {
    form.value.second_choice = v === NO_CHOICE ? '' : v
  },
})

/** 打开弹窗时刷新部门名单（拿不到就只保留现有取值可选项）。 */
watch(
  () => props.open,
  (open) => {
    if (!open) return
    void departmentApi
      .list()
      .then(({ items }) => {
        departments.value = items.map((d) => d.name)
      })
      .catch(() => {
        departments.value = []
      })
  },
  { immediate: true },
)

// 每次打开按目标候选人当前值初始化（列表刷新后对象会换，故监听 ref 值本身）。
watch(
  () => [props.open, props.candidate] as const,
  ([open, c]) => {
    if (!open || !c) return
    form.value = {
      first_choice: c.first_choice ?? '',
      second_choice: c.second_choice ?? '',
      accept_adjust: c.accept_adjust ?? false,
    }
    saving.value = false
  },
  { immediate: true },
)

async function submit(): Promise<void> {
  const c = props.candidate
  if (!c || saving.value) return
  saving.value = true
  try {
    await candidateApi.updatePreferences(c.id, {
      first_choice: form.value.first_choice.trim(),
      second_choice: form.value.second_choice.trim(),
      accept_adjust: form.value.accept_adjust,
    })
    emit('update:open', false)
    toast.success(`已更新「${c.name}」的志愿与调剂`)
    emit('saved')
  } catch (e) {
    toastError(e)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <FormDialog
    :open="open"
    :title="'修改志愿与调剂'"
    submit-text="保存"
    size="sm"
    :loading="saving"
    @update:open="emit('update:open', $event)"
    @submit="submit"
  >
    <div class="grid gap-3">
      <div class="grid gap-2">
        <Label :for="`${uid}-first`">第一志愿</Label>
        <Select v-model="firstChoice">
          <SelectTrigger :id="`${uid}-first`" class="w-full" aria-label="第一志愿">
            <SelectValue placeholder="选择部门" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem :value="NO_CHOICE">无</SelectItem>
            <SelectItem v-for="name in choiceOptions" :key="name" :value="name">{{ name }}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div class="grid gap-2">
        <Label :for="`${uid}-second`">第二志愿</Label>
        <Select v-model="secondChoice">
          <SelectTrigger :id="`${uid}-second`" class="w-full" aria-label="第二志愿">
            <SelectValue placeholder="选择部门" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem :value="NO_CHOICE">无</SelectItem>
            <SelectItem v-for="name in choiceOptions" :key="name" :value="name">{{ name }}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <label class="flex items-center gap-2 text-sm" :for="`${uid}-adjust`">
        <input
          :id="`${uid}-adjust`"
          v-model="form.accept_adjust"
          type="checkbox"
          class="size-4 accent-primary"
        />
        接受调剂
      </label>
    </div>
  </FormDialog>
</template>
