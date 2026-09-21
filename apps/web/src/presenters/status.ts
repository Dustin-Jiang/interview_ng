import type { Component } from 'vue'
import { Gavel, UserCheck, UserSearch, UsersRound } from 'lucide-vue-next'

import type { BadgeVariants } from '@/components/ui/badge'
import type { AdmissionOutcome } from '@/domain/admission'
import type { AdmissionStatus, CandidateStatus, SystemPhase } from '@/models'

/** 房间派生状态的"空闲"哨兵键（无候选人的空房间）。 */
export const ROOM_EMPTY = 'ROOM_EMPTY'

/** 候选人状态的展示元数据：中文标签 + 徽章变体（View 层展示逻辑，与业务数据解耦）。 */
export const STATUS_PRESENTATION: Record<
  CandidateStatus,
  { label: string; badge: NonNullable<BadgeVariants['variant']> }
> = {
  NOT_CHECKED_IN: { label: '未签到', badge: 'outline' },
  CHECKED_IN_PENDING_ASSIGN: { label: '排队中', badge: 'secondary' },
  ASSIGNED: { label: '待面试', badge: 'secondary' },
  IN_PROGRESS: { label: '面试中', badge: 'default' },
  COMPLETED: { label: '面试已结束', badge: 'destructive' },
  ADMISSION_PENDING: { label: '待录取', badge: 'secondary' },
  ADMITTED: { label: '已录取', badge: 'default' },
}

/** 空房间派生状态展示。 */
export const EMPTY_PRESENTATION = { label: '空闲', badge: 'outline' } as const

/** 系统阶段展示元数据：中文标签 + 图标（捡漏页与系统状态页共用，避免两处标签漂移）。 */
export const PHASE_PRESENTATION: Record<SystemPhase, { label: string; icon: Component }> = {
  interview: { label: '面试阶段', icon: UsersRound },
  admission: { label: '录取阶段', icon: UserCheck },
  leftover: { label: '捡漏阶段', icon: UserSearch },
  settlement: { label: '结算阶段', icon: Gavel },
}

/** 录取决定状态的展示元数据：中文标签 + 徽章变体。 */
export const ADMISSION_PRESENTATION: Record<
  AdmissionStatus,
  { label: string; badge: NonNullable<BadgeVariants['variant']> }
> = {
  pending: { label: '待定', badge: 'outline' },
  admitted: { label: '录取', badge: 'default' },
  withdrawn: { label: '放弃', badge: 'secondary' },
}

/** 录取汇总结论的展示元数据：admitted 档文案拼接部门名（录取到 X）。 */
export function admissionOutcomePresentation(
  o: AdmissionOutcome,
): { label: string; badge: NonNullable<BadgeVariants['variant']> } {
  switch (o.kind) {
    case 'admitted':
      return { label: `录取到 ${o.departmentName}`, badge: 'default' }
    case 'none':
      return { label: '未录取', badge: 'outline' }
    case 'conflict':
      return { label: '多家录取', badge: 'destructive' }
    case 'pending':
      return { label: '待定', badge: 'outline' }
  }
}