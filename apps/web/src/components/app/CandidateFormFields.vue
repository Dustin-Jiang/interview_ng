<!--
  CandidateFormFields —— 候选人资料表单字段（学号 / 姓名 / 志愿 / 调剂 / 联系方式 / 简介），
  新增与编辑对话框共用。整体以 `v-model:info` 绑定资料对象（CandidateInfoPayload），
  每次修改不可变地替换整个对象（defineModel 的写法，便于父级 watch 到变化）；
  文本字段 Enter 与对话框提交按钮同一入口。
-->
<script setup lang="ts">
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import type { CandidateInfoPayload } from '@/models'

const props = defineProps<{
  /** 字段 id 前缀（同一页面内新增/编辑两个对话框各有独立 id）。 */
  idPrefix: string
}>()

const info = defineModel<CandidateInfoPayload>('info', { required: true })

const emit = defineEmits<{ submit: [] }>()

/** 文本字段绑定：读当前值，写则整体替换（未填按空串）。 */
function text(key: keyof CandidateInfoPayload) {
  return {
    modelValue: String(info.value[key] ?? ''),
    'onUpdate:modelValue': (v: string | number) => {
      info.value = { ...info.value, [key]: String(v) }
    },
  }
}

/** 布尔字段绑定（复选框）：写则整体替换。 */
function checked(key: keyof CandidateInfoPayload) {
  return {
    checked: info.value[key] === true,
    onChange: (e: Event) => {
      info.value = { ...info.value, [key]: (e.target as HTMLInputElement).checked }
    },
  }
}
function submitOnEnter() {
  emit('submit')
}
</script>

<template>
  <div class="grid gap-2">
    <Label :for="`${props.idPrefix}-student-no`">学号</Label>
    <Input
      :id="`${props.idPrefix}-student-no`"
      v-bind="text('student_no')"
      inputmode="numeric"
      placeholder="纯数字学号"
      @keydown.enter="submitOnEnter"
    />
  </div>
  <div class="grid gap-2">
    <Label :for="`${props.idPrefix}-name`">姓名</Label>
    <Input
      :id="`${props.idPrefix}-name`"
      v-bind="text('name')"
      placeholder="候选人姓名"
      @keydown.enter="submitOnEnter"
    />
  </div>
  <div class="grid gap-4 sm:grid-cols-2">
    <div class="grid gap-2">
      <Label :for="`${props.idPrefix}-first-choice`">第一志愿</Label>
      <Input
        :id="`${props.idPrefix}-first-choice`"
        v-bind="text('first_choice')"
        placeholder="专业 / 方向（可选）"
        @keydown.enter="submitOnEnter"
      />
    </div>
    <div class="grid gap-2">
      <Label :for="`${props.idPrefix}-second-choice`">第二志愿</Label>
      <Input
        :id="`${props.idPrefix}-second-choice`"
        v-bind="text('second_choice')"
        placeholder="专业 / 方向（可选）"
        @keydown.enter="submitOnEnter"
      />
    </div>
  </div>
  <div class="grid gap-4 sm:grid-cols-2">
    <div class="grid gap-2">
      <Label :for="`${props.idPrefix}-phone`">手机号</Label>
      <Input
        :id="`${props.idPrefix}-phone`"
        v-bind="text('phone')"
        type="tel"
        placeholder="（可选）"
        @keydown.enter="submitOnEnter"
      />
    </div>
    <div class="grid gap-2">
      <Label :for="`${props.idPrefix}-qq`">QQ 号</Label>
      <Input
        :id="`${props.idPrefix}-qq`"
        v-bind="text('qq')"
        inputmode="numeric"
        placeholder="（可选）"
        @keydown.enter="submitOnEnter"
      />
    </div>
  </div>
  <div class="grid gap-2">
    <Label :for="`${props.idPrefix}-email`">邮箱</Label>
    <Input
      :id="`${props.idPrefix}-email`"
      v-bind="text('email')"
      type="email"
      placeholder="（可选）"
      @keydown.enter="submitOnEnter"
    />
  </div>
  <label
    class="flex min-h-11 cursor-pointer items-center gap-2 text-sm font-medium"
    :for="`${props.idPrefix}-accept-adjust`"
  >
    <input
      :id="`${props.idPrefix}-accept-adjust`"
      type="checkbox"
      class="size-4 accent-primary"
      v-bind="checked('accept_adjust')"
    />
    接受调剂
  </label>
  <div class="grid gap-2">
    <Label :for="`${props.idPrefix}-profile`">个人简介</Label>
    <Textarea
      :id="`${props.idPrefix}-profile`"
      v-bind="text('profile')"
      rows="3"
      placeholder="技术栈 / 背景（可选）"
    />
  </div>
</template>
