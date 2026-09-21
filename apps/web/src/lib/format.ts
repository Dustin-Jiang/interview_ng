/** 展示格式化工具（View 消费，不影响 Model 数据）。 */

/** 姓名首字母（头像回退位）：两段及以上取前两段首字，否则取前两个字。 */
export function initialsOf(name: string): string {
  const trimmed = name.trim()
  const parts = trimmed.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return (trimmed.slice(0, 2) || '?').toUpperCase()
}

export function formatDateTime(value?: string): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}
