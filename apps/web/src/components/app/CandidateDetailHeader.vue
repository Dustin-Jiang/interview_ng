<!--
  CandidateDetailHeader —— 详情区顶部资料卡（头像 + 姓名 + 一支徽章 + 简介 + 动作）。
  收编「候选人查看」与「捡漏竞拍」详情卡头部；徽章数据源（状态 / 成交结果）与
  右侧动作按钮（进入房间 / 结算）由调用方通过 props 与 #actions 提供，
  卡片正文由默认插槽填充。
-->
<script setup lang="ts">
import { Avatar, AvatarFallback, avatarVariants } from '@/components/ui/avatar'
import { Badge, type BadgeVariants } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { initialsOf } from '@/lib/format'

const props = withDefaults(
  defineProps<{
    candidate: { name: string; profile?: string | null }
    /** 姓名旁的单支徽章（候选人状态或成交结果）。 */
    badge?: { label: string; variant: NonNullable<BadgeVariants['variant']> } | null
    /** 无简介时的占位文案。 */
    profileFallback?: string
  }>(),
  { badge: null, profileFallback: '暂无个人简介' },
)
</script>

<template>
  <Card>
    <CardHeader class="flex-row items-start gap-4 space-y-0">
      <Avatar :class="avatarVariants({ size: 'lg' })" aria-hidden="true">
        <AvatarFallback>{{ initialsOf(props.candidate.name) }}</AvatarFallback>
      </Avatar>
      <div class="min-w-0 flex-1 space-y-1.5">
        <div class="flex flex-wrap items-center gap-2">
          <CardTitle class="truncate text-xl">{{ props.candidate.name }}</CardTitle>
          <Badge v-if="props.badge" :variant="props.badge.variant" class="text-sm">{{ props.badge.label }}</Badge>
        </div>
        <p class="whitespace-pre-line text-sm text-muted-foreground">
          {{ props.candidate.profile || props.profileFallback }}
        </p>
      </div>
      <slot name="actions" />
    </CardHeader>
    <CardContent class="pt-0">
      <slot />
    </CardContent>
  </Card>
</template>
