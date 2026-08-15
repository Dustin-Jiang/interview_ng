/**
 * 应用配置。
 * 身份不再写死：登录态由 useAuth 从 localStorage 恢复（token 7 天有效期），
 * 当前用户与权限经 GET /api/me 获取，权限驱动 UI 显隐。
 */
