/**
 * 房间展示名 —— 全应用统一口径（纯函数层，无 Vue 依赖）。
 * 房间名是可选别名：**任何界面都不再显示房间编号**，未命名统一显示「未命名」。
 */

/** 未命名房间的展示占位。 */
export const UNNAMED_ROOM_LABEL = '未命名'

/** 取值形状：任何带 name 的房间快照（`Room` 或其子集）都可直接传入。 */
export type RoomLabelSource = { name?: string | null }

/**
 * 房间展示名：入参是**房间对象**（不是 name 字符串），未命名 → 「未命名」。
 * 对非字符串 name（数据异常/旧模块混用）退化为「未命名」而不抛错——展示函数不该让页面崩。
 */
export function roomLabel(room?: RoomLabelSource | null): string {
  const name = room?.name
  return (typeof name === 'string' ? name.trim() : '') || UNNAMED_ROOM_LABEL
}
