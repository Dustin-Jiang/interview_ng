/**
 * DOM 小工具（无 Vue 依赖）。
 */

/**
 * 事件目标是否为可编辑控件（input / textarea / select / contenteditable）。
 * 全局方向键处理用它让位：焦点在输入控件里时，方向键归控件自己（光标、数字步进等）。
 */
export function isEditableTarget(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el) return false
  return (
    el.tagName === 'INPUT' ||
    el.tagName === 'TEXTAREA' ||
    el.tagName === 'SELECT' ||
    el.isContentEditable
  )
}

/**
 * 事件目标是否为「Enter 应激活它」的控件（按钮 / 链接）。
 * 名册项是 `button[role="option"]`：它按 Enter 不算激活（捡漏页把它留给「保存报价」），故单独排除。
 */
export function isActivatableElement(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null
  if (!el) return false
  if (el.getAttribute('role') === 'option') return false
  return el.tagName === 'BUTTON' || el.tagName === 'A' || el.getAttribute('role') === 'button'
}

/** 是否有模态弹窗/浮层打开（页面级快捷键在弹窗内应完全让位）。 */
export function isModalOpen(): boolean {
  return document.querySelector('[role="dialog"], [role="alertdialog"]') !== null
}
