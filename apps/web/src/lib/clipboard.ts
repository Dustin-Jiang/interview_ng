/**
 * 剪贴板写入（无 Vue 依赖）。
 *
 * 为什么不能只用 `navigator.clipboard`：它**只在安全上下文**可用（https / localhost）。
 * 本项目的部署形态是单镜像在 `:8080` 上直供前端（README「部署（容器，单进程单镜像）」），
 * 从别的机器用 `http://<内网IP>:8080` 打开时 `navigator.clipboard` 是 undefined，
 * 直接调用会抛错——复制按钮在那台机器上静默失效。故退回 `document.execCommand('copy')`
 * + 临时 textarea（老 API，但非安全上下文仍可用）。
 *
 * @returns 是否真的复制成功——调用方据此决定成功 / 失败文案，不要假设一定成功。
 */
export async function copyText(text: string): Promise<boolean> {
  // 有 API 时优先用它（能保留富文本语义、不受选区影响）
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 有 API 但被拒（权限 / 非用户手势 / 非聚焦文档）→ 继续走兜底，别直接报错
    }
  }
  return legacyCopy(text)
}

/** 兜底：临时 textarea + `execCommand('copy')`（含选中与清理）。 */
function legacyCopy(text: string): boolean {
  const el = document.createElement('textarea')
  el.value = text
  // 不能 display:none（那样不可选中）；移到视口外且不产生滚动
  el.setAttribute('readonly', '')
  el.style.position = 'fixed'
  el.style.top = '-1000px'
  el.style.opacity = '0'
  document.body.appendChild(el)
  try {
    el.select()
    return document.execCommand('copy')
  } catch {
    return false
  } finally {
    el.remove()
  }
}
