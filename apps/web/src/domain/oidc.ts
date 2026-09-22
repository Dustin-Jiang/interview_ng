/**
 * OIDC 登录错误码 → 中文提示（domain 层：纯函数，无请求、无 UI 状态、不引第三方库）。
 * 后端在授权/回调失败时 302 回 `/login?oidc_error=<code>`，登录页据此展示原因。
 */
export const OIDC_ERROR_MESSAGES: Record<string, string> = {
  oidc_not_configured: '统一身份认证尚未配置完成',
  oidc_discovery_failed: '无法连接身份提供方，请检查 Issuer 配置',
  oidc_state_invalid: '登录请求已失效，请重新发起',
  oidc_exchange_failed: '身份提供方拒绝了本次登录',
  oidc_nonce_invalid: '登录校验失败，请重新发起',
  oidc_claims_invalid: '身份提供方返回的凭据无法识别',
  oidc_role_unmapped: '账号未匹配到任何角色，请联系管理员',
  oidc_user_unknown: '账号尚未开通，请联系管理员',
  oidc_username_taken: '用户名已被占用，请联系管理员',
  oidc_login_failed: '统一身份认证登录失败',
}

/** 未知错误码回退到通用文案。 */
export function oidcErrorMessage(code: string): string {
  return OIDC_ERROR_MESSAGES[code] ?? OIDC_ERROR_MESSAGES.oidc_login_failed
}

// ---- 回调地址（主机可编辑，路径固定） ----

/** 是否 http(s) 来源（其余协议不是合法回调主机）。 */
function httpOrigin(u: URL): string {
  return u.protocol === 'http:' || u.protocol === 'https:' ? u.origin : ''
}

/**
 * 取已保存回调地址的主机部分（`scheme://host[:port]`）；空/脏数据 → `""`。
 * 路径不参与编辑：`OIDC_CALLBACK_PATH` 固定，管理员只填主机。
 */
export function callbackOriginOf(url: string): string {
  try {
    return httpOrigin(new URL(url.trim()))
  } catch {
    return ''
  }
}

/**
 * 归一化管理员输入的回调主机：接受主机（可带端口），粘贴完整 URL 时只取 origin
 * （路径由固定后缀决定，多余的路径/查询/锚点一律丢弃；协议大小写与默认端口按 URL 规范归一）。
 * 非法（缺 `http(s)://`、空、非 http 协议）→ `null`，由调用方提示。
 */
export function normalizeCallbackOrigin(raw: string): string | null {
  const text = raw.trim()
  if (!text) return null
  try {
    const u = new URL(text)
    return u.hostname ? httpOrigin(u) || null : null
  } catch {
    return null
  }
}
