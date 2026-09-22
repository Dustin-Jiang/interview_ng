/**
 * useTheme —— 深色模式主题（函数式 ViewModel，模块级单例，与 useSystemStatus / useBoardChannel 同款）。
 *
 * 数据流：用户模式（localStorage，跨标签页同步）→ `resolveTheme(mode, 系统偏好)` →
 * 落到 `<html>` 的 `.dark` 类（`.dark` 覆盖 `:root` 的语义变量，见 assets/index.css）。
 * 因此**所有界面只要用语义 token 就自动跟随**，组件里不需要写 `dark:` 变体。
 *
 * 首屏由 `index.html` 的内联脚本按同一规则先行应用（避免白屏闪烁）；
 * 模块级副作用在导入时立即再执行一次，两者结果一致。
 */
import { computed, watchEffect, type ComputedRef, type Ref } from 'vue'
import { usePreferredDark, useStorage } from '@vueuse/core'

import {
  THEME_STORAGE_KEY,
  parseThemeMode,
  resolveTheme,
  type ResolvedTheme,
  type ThemeMode,
} from '@/domain/theme'

/** 模块级单例：原始存储值（`listenToStorageChanges` 让其它标签页的改动同步过来）。 */
const stored = useStorage<string>(THEME_STORAGE_KEY, 'system', undefined, { listenToStorageChanges: true })
const systemPrefersDark = usePreferredDark()

/** 用户选择的模式（脏数据回落 system）。 */
const mode = computed<ThemeMode>(() => parseThemeMode(stored.value))
/** 实际生效主题。 */
const resolved = computed<ResolvedTheme>(() => resolveTheme(mode.value, systemPrefersDark.value))

// 模块级副作用：解析结果落到 <html> 的 .dark 类（`.dark` 一挂上，全站语义 token 即切换）。
watchEffect(() => {
  document.documentElement.classList.toggle('dark', resolved.value === 'dark')
})

export interface UseTheme {
  /** 用户选择的模式（浅色 / 深色 / 跟随系统）。 */
  readonly mode: ComputedRef<ThemeMode>
  /** 实际生效主题（`<html>.dark` 的现状）。 */
  readonly resolved: ComputedRef<ResolvedTheme>
  /** 系统是否偏好深色（`mode` 为 system 时的来源）。 */
  readonly systemPrefersDark: Ref<boolean>
  setMode: (next: ThemeMode) => void
}

export function useTheme(): UseTheme {
  function setMode(next: ThemeMode): void {
    stored.value = next
  }

  return { mode, resolved, systemPrefersDark, setMode }
}
