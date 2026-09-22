/**
 * 深色模式主题的纯逻辑（domain 层：无 Vue / DOM 依赖）。
 *
 * 三档模式：`light` / `dark` / `system`（默认跟随系统）。
 * 解析结果（`light` / `dark`）由 `composables/useTheme.ts` 落到 `<html>` 的 `.dark` 类上，
 * 对应 `assets/index.css` 的 `.dark` 语义变量覆盖；`color-scheme` 随该类切换，
 * 原生表单控件与原生滚动条因此一同跟随（见 index.css）。
 *
 * ⚠️ `index.html` 的首屏内联脚本按**同一套存储键与解析规则**先行应用主题（防白屏闪烁）。
 * 内联脚本不能 import，改动本文件的键名或规则时**必须同步改 `index.html`**。
 */

/** 主题存储键（首屏内联脚本共用，不可随意改名）。 */
export const THEME_STORAGE_KEY = 'interview_ng_theme'

/** 用户选择的主题模式。 */
export type ThemeMode = 'light' | 'dark' | 'system'

/** 实际生效的主题（决定 `<html>` 是否带 `.dark`）。 */
export type ResolvedTheme = 'light' | 'dark'

/**
 * 归一化存储值：仅接受三个合法模式，其它（缺失/旧值/手改脏数据）一律回落 `system`
 * ——跟随系统是最安全的默认，不会把用户锁在错误主题里。
 */
export function parseThemeMode(raw: unknown): ThemeMode {
  return raw === 'light' || raw === 'dark' || raw === 'system' ? raw : 'system'
}

/** 由「模式 + 系统偏好」得到实际生效主题。 */
export function resolveTheme(mode: ThemeMode, systemPrefersDark: boolean): ResolvedTheme {
  if (mode === 'system') return systemPrefersDark ? 'dark' : 'light'
  return mode
}
