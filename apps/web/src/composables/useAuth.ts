/**
 * useAuth —— 全局登录态组合式函数（函数式 ViewModel，无 Pinia）。
 * 职责：
 *  - 登录：调 POST /api/sessions，token 存 localStorage（7 天，刷新不丢）；
 *  - 启动恢复：读 token 并调 GET /api/me 拉取用户/角色/权限并集；
 *  - 权限驱动 UI 的输入：hasPermission(perm)；
 *  - 注册 HTTP 401 处理器（登出 + 跳登录页）。
 */
import { computed, ref, type ComputedRef, type Ref } from 'vue'
import { useRouter } from 'vue-router'
import { authApi, oidcApi, setAuthToken, setUnauthorizedHandler } from '@/api/http'
import type { Permission, User, UserProfile } from '@/models'

const TOKEN_KEY = 'interview_ng_token'

/** 模块级单例状态：登录态全局唯一（由 App.vue 在 setup 中初始化）。 */
const profile = ref<UserProfile | null>(null)
const booting = ref(true)
/** 后端是否启用统一身份认证（登录页据此显示入口；读取失败按未启用处理）。 */
const oidcEnabled = ref(false)
/** token 是否已从 localStorage 恢复过（与 401 处理器注册标志分离，避免互相阻塞）。 */
let tokenRestored = false
/** 401 处理器是否已注册。 */
let handlerRegistered = false

function persistToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
  setAuthToken(token)
}

function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
  setAuthToken('')
}

/** 读取并恢复本地 token（页面刷新时）。注意：必须在 useAuth() 之前/之后都能可靠执行，
 *  不能被 useAuth() 的处理器注册标志提前短路——App.vue setup 先于路由守卫运行。 */
export function initAuth(): void {
  if (tokenRestored) return
  tokenRestored = true
  const saved = localStorage.getItem(TOKEN_KEY)
  if (saved) setAuthToken(saved)
}

/** 等待登录态初始化完成（路由守卫使用）。 */
export async function ensureAuthReady(): Promise<boolean> {
  initAuth()
  if (profile.value) return true
  const saved = localStorage.getItem(TOKEN_KEY)
  if (!saved) {
    booting.value = false
    return false
  }
  try {
    profile.value = await authApi.me()
    return true
  } catch {
    // 401 由 axios 拦截器触发登出（清 token）；此处不主动清 token，避免网络抖动误删登录态。
    return false
  } finally {
    booting.value = false
  }
}

export interface UseAuth {
  readonly user: Ref<User | null>
  readonly currentUserId: ComputedRef<number | null>
  readonly roles: Ref<string[]>
  readonly permissions: Ref<Permission[]>
  readonly isLoggedIn: ComputedRef<boolean>
  /** 后端是否启用统一身份认证（登录页据此渲染入口）。 */
  readonly oidcEnabled: Ref<boolean>
  hasPermission: (perm: Permission) => boolean
  login: (username: string, password: string) => Promise<void>
  /** 读取登录方式开关（失败按「未启用 OIDC」处理，不阻塞密码登录）。 */
  loadAuthenticationOptions: () => Promise<void>
  /** 用 OIDC 回调带回的一次性登录码换取会话。 */
  completeOidc: (code: string) => Promise<void>
  logout: () => void
}

export function useAuth(): UseAuth {
  const router = useRouter()

  const user = computed<User | null>(() => profile.value?.user ?? null)
  const currentUserId = computed<number | null>(() => user.value?.id ?? null)
  const roles = computed<string[]>(() => profile.value?.roles ?? [])
  const permissions = computed<Permission[]>(() => profile.value?.permissions ?? [])
  const isLoggedIn = computed(() => !!profile.value)

  function hasPermission(perm: Permission): boolean {
    return permissions.value.includes(perm)
  }

  /** 落地会话：持久化 token 并写入登录态（密码登录与 OIDC 兑换共用）。 */
  function adoptSession(res: UserProfile & { token: string }): void {
    persistToken(res.token)
    profile.value = { user: res.user, roles: res.roles, permissions: res.permissions }
  }

  async function login(username: string, password: string): Promise<void> {
    adoptSession(await authApi.login(username, password))
  }

  async function loadAuthenticationOptions(): Promise<void> {
    try {
      oidcEnabled.value = (await oidcApi.options()).oidc.enabled
    } catch {
      oidcEnabled.value = false
    }
  }

  async function completeOidc(code: string): Promise<void> {
    adoptSession(await oidcApi.createSession(code))
  }

  function logout(): void {
    clearToken()
    profile.value = null
    void router.push({ name: 'login' })
  }

  // 注册全局 401 处理器：任何接口 401 即登出跳登录页。
  // 用独立标志，不影响 initAuth 的 token 恢复。
  if (!handlerRegistered) {
    handlerRegistered = true
    setUnauthorizedHandler(logout)
  }

  return {
    user,
    currentUserId,
    roles,
    permissions,
    isLoggedIn,
    oidcEnabled,
    hasPermission,
    login,
    loadAuthenticationOptions,
    completeOidc,
    logout,
  }
}
