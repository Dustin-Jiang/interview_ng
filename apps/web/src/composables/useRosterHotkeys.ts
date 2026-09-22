/**
 * useRosterHotkeys —— 名册页（候选人查看 / 捡漏竞拍）共用的 window 快捷键。
 * 两个名册页统一 **↑/↓ 切换候选人**；键位守卫与注册/注销生命周期收在这里，
 * 页面只需提供 goPrev / goNext 与「↑/↓ 之外」的按键处理：
 *  - 通用守卫：事件已被内层控件处理（`defaultPrevented`）、有模态弹窗打开、
 *    焦点在输入控件内 → 完全让位；
 *  - `isHotkeyInput` 指定的控件内仍响应方向键（捡漏页的出价框：只放数字、无光标需求）；
 *  - `onOtherKey` 处理其余键（捡漏页的 ←/→ 调价、Enter 保存；候选人页的 1/2/3 决定）。
 */
import { onBeforeUnmount, onMounted } from 'vue'

import { isEditableTarget, isModalOpen } from '@/lib/dom'

export interface RosterHotkeyOptions {
  goPrev: () => void
  goNext: () => void
  /** 该控件内仍响应方向键（如捡漏页标记 `data-bid-input` 的出价输入框）。 */
  isHotkeyInput?: (target: EventTarget | null) => boolean
  /** ↑/↓ 之外的按键（通用守卫已通过）；返回 true 表示已消费（阻止默认行为）。 */
  onOtherKey?: (e: KeyboardEvent, inHotkeyInput: boolean) => boolean
}

export function useRosterHotkeys(opts: RosterHotkeyOptions): void {
  function onKeydown(e: KeyboardEvent): void {
    if (e.defaultPrevented || isModalOpen()) return
    const inHotkeyInput = opts.isHotkeyInput?.(e.target) ?? false
    if (!inHotkeyInput && isEditableTarget(e.target)) return // 搜索框等输入控件：让位

    if (e.key === 'ArrowUp') {
      e.preventDefault()
      opts.goPrev()
      return
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      opts.goNext()
      return
    }
    if (opts.onOtherKey?.(e, inHotkeyInput)) e.preventDefault()
  }

  onMounted(() => window.addEventListener('keydown', onKeydown))
  onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
}
