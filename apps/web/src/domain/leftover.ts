/**
 * 捡漏竞拍领域 —— 纯函数层。捡漏页与系统状态页共用的出价派生口径。
 */
import type { Bid } from '@/models'

/**
 * 候选人 → 各部门出价，每组按金额降序（管理端可见全部门）。
 * 纯函数：不改入参，返回新 Map。
 */
export function groupBidsByCandidate(bids: readonly Bid[]): Map<number, Bid[]> {
  const map = new Map<number, Bid[]>()
  for (const b of bids) {
    const list = map.get(b.candidate_id) ?? []
    list.push(b)
    map.set(b.candidate_id, list)
  }
  for (const list of map.values()) list.sort((x, y) => y.amount - x.amount)
  return map
}
