<!--
  CandidateFormFields —— 候选人资料表单字段（学号 / 姓名 / 简介），新增与编辑对话框共用。
  三个字段均为 v-model（student-no / name / profile）；Enter 与对话框提交按钮同一入口。
-->
<script setup lang="ts">
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

const props = defineProps<{
  /** 字段 id 前缀（同一页面内新增/编辑两个对话框各有独立 id）。 */
  idPrefix: string
}>()

const studentNo = defineModel<string>('studentNo', { required: true })
const name = defineModel<string>('name', { required: true })
const profile = defineModel<string>('profile', { required: true })

const emit = defineEmits<{ submit: [] }>()
</script>

<template>
  <div class="grid gap-2">
    <Label :for="`${props.idPrefix}-student-no`">学号</Label>
    <Input
      :id="`${props.idPrefix}-student-no`"
      v-model="studentNo"
      inputmode="numeric"
      placeholder="纯数字学号"
      @keydown.enter="emit('submit')"
    />
  </div>
  <div class="grid gap-2">
    <Label :for="`${props.idPrefix}-name`">姓名</Label>
    <Input
      :id="`${props.idPrefix}-name`"
      v-model="name"
      placeholder="候选人姓名"
      @keydown.enter="emit('submit')"
    />
  </div>
  <div class="grid gap-2">
    <Label :for="`${props.idPrefix}-profile`">个人简介</Label>
    <Textarea
      :id="`${props.idPrefix}-profile`"
      v-model="profile"
      rows="3"
      placeholder="技术栈 / 背景（可选）"
    />
  </div>
</template>
