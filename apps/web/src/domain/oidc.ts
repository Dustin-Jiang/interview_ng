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
