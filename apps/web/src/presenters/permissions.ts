/**
 * 权限分组展示 —— 仅负责「管理/流程」归类与排序（View 层展示逻辑，不汉化）。
 */
import { PERMISSIONS } from '@/models'

export type PermissionGroup = '管理' | '流程'

/** 权限 key → 展示分组；未知 key 返回 null。 */
export function permissionGroup(perm: string): PermissionGroup | null {
  switch (perm) {
    case PERMISSIONS.USERS_MANAGE:
    case PERMISSIONS.CANDIDATES_MANAGE:
    case PERMISSIONS.CANDIDATES_BROWSE_ALL:
    case PERMISSIONS.ROOMS_MANAGE:
      return '管理'
    case PERMISSIONS.CANDIDATES_CREATE:
    case PERMISSIONS.CANDIDATES_CHECKIN:
    case PERMISSIONS.CANDIDATES_ASSIGN:
    case PERMISSIONS.ROOMS_VIEW:
    case PERMISSIONS.ROOMS_CHAT:
    case PERMISSIONS.ROOMS_MOVE_PHASE:
      return '流程'
    default:
      return null
  }
}

/** 按官方目录顺序排列的权限 key（用于排序与全量清单）。 */
export const PERMISSION_ORDER = [
  PERMISSIONS.USERS_MANAGE,
  PERMISSIONS.CANDIDATES_MANAGE,
  PERMISSIONS.CANDIDATES_BROWSE_ALL,
  PERMISSIONS.ROOMS_MANAGE,
  PERMISSIONS.CANDIDATES_CREATE,
  PERMISSIONS.CANDIDATES_CHECKIN,
  PERMISSIONS.CANDIDATES_ASSIGN,
  PERMISSIONS.ROOMS_VIEW,
  PERMISSIONS.ROOMS_CHAT,
  PERMISSIONS.ROOMS_MOVE_PHASE,
]

/** 是否持有任一管理类权限（设置页入口与侧栏分区的显隐前提）。 */
export function hasAnyManagePermission(perms: readonly string[]): boolean {
  return [
    PERMISSIONS.USERS_MANAGE,
    PERMISSIONS.CANDIDATES_MANAGE,
    PERMISSIONS.CANDIDATES_CREATE,
    PERMISSIONS.CANDIDATES_CHECKIN,
  ].some((p) => perms.includes(p))
}

/** 将一组权限 key 按「管理/流程」分组有序返回（无权限的组为空数组）。 */
export function groupPermissionEntries(perms: readonly string[]): Record<PermissionGroup, string[]> {
  const set = new Set(perms)
  const out: Record<PermissionGroup, string[]> = { 管理: [], 流程: [] }
  for (const p of PERMISSION_ORDER) {
    if (!set.has(p)) continue
    const g = permissionGroup(p)
    if (g) out[g].push(p)
  }
  return out
}
