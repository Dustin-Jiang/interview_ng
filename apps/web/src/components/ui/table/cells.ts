/**
 * cells —— DataTable 单元格渲染辅助（h() 构建，各列表共用同一展示口径）。
 * 空值统一以 `-` 占位；配合「各列 nowrap、放不下由表格容器横向滚动」的列表约定。
 */
import { h, type VNode } from 'vue'

/** 单元格文本：空值以 `-` 占位，非空按给定类渲染（whitespace / 字体等由调用方给）。 */
export function textCell(value: string, classNames: string): VNode {
  return value ? h('div', { class: classNames }, value) : h('span', { class: 'text-muted-foreground' }, '-')
}
