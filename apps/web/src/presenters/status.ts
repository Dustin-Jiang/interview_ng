import type { BadgeVariants } from '@/components/ui/badge'
import type { CandidateStatus } from '@/models'

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
  COMPLETED: { label: '已结束', badge: 'destructive' },
}

/** 空房间派生状态展示。 */
export const EMPTY_PRESENTATION = { label: '空闲', badge: 'outline' } as const