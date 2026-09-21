import { createRouter, createWebHistory } from 'vue-router'
import { ensureAuthReady } from '@/composables/useAuth'

/**
 * 路由表 —— 与后端资源路径保持同构的 RESTful 形式：
 * 集合用复数名词（`/candidates`、`/rooms`），条目用 `/:id`（`/candidates/:candidateId`）；
 * 无动词路径；页面内的筛选/视图态留在 query（`?q=`、`?status=`）。
 * 旧路径保留重定向，保证既有书签与分享链接可用。
 */
export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
    },
    {
      // 首页（个人信息 + 快捷入口）。
      path: '/',
      name: 'home',
      component: () => import('@/views/WelcomeView.vue'),
    },
    {
      // 候选人集合（名册 + 详情窗格，未选中具体候选人）。
      path: '/candidates',
      name: 'candidates',
      component: () => import('@/views/CandidateRecordsView.vue'),
    },
    {
      // 候场大屏：候选人的候场视图（须声明在 /candidates/:candidateId 之前）。
      path: '/candidates/waiting',
      name: 'candidates-waiting',
      component: () => import('@/views/WaitingBoardView.vue'),
    },
    {
      // 候选人参目：名册 + 该候选人的资料与面试记录（深链可分享）。
      path: '/candidates/:candidateId(\\d+)',
      name: 'candidate',
      component: () => import('@/views/CandidateRecordsView.vue'),
    },
    {
      // 房间集合（列表）。
      path: '/rooms',
      name: 'rooms',
      component: () => import('@/views/RoomView.vue'),
    },
    {
      // 房间条目（实时聊天界面）。
      path: '/rooms/:roomId(\\d+)',
      name: 'room',
      component: () => import('@/views/RoomView.vue'),
    },
    {
      // 捡漏竞拍命名空间根（总览 + 候选人名册）。
      path: '/leftover',
      name: 'leftover',
      component: () => import('@/views/LeftoverView.vue'),
    },
    {
      // 捡漏候选人条目（名册 + 该候选人的出价与结算详情）。
      path: '/leftover/candidates/:candidateId(\\d+)',
      name: 'leftover-candidate',
      component: () => import('@/views/LeftoverView.vue'),
    },
    {
      // 设置：管理功能左右分栏（子路径即各自资源）。
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/SettingsView.vue'),
      children: [
        {
          path: 'candidates',
          name: 'settings-candidates',
          component: () => import('@/views/SettingsCandidatesView.vue'),
        },
        {
          path: 'users',
          name: 'settings-users',
          component: () => import('@/views/SettingsUsersView.vue'),
        },
        {
          path: 'roles',
          name: 'settings-roles',
          component: () => import('@/views/SettingsRolesView.vue'),
        },
        {
          path: 'departments',
          name: 'settings-departments',
          component: () => import('@/views/SettingsDepartmentsView.vue'),
        },
        {
          path: 'system/status',
          name: 'settings-system-status',
          component: () => import('@/views/SettingsSystemStatusView.vue'),
        },
      ],
    },
    // ---- 旧路径重定向（书签/分享兼容；新路径见上方路由） ----
    { path: '/waiting', redirect: { name: 'candidates-waiting' } },
    { path: '/room', redirect: { name: 'rooms' } },
    { path: '/room/:roomId', redirect: (to) => `/rooms/${to.params.roomId}` },
    { path: '/settings/system-status', redirect: { name: 'settings-system-status' } },
    { path: '/candidates/manage', redirect: { name: 'settings-candidates' } },
    { path: '/users', redirect: { name: 'settings-users' } },
  ],
})

// 全局守卫：未登录访问非 /login → 跳登录；已登录访问 /login → 跳欢迎页。
router.beforeEach(async (to) => {
  const authed = await ensureAuthReady()
  if (to.name === 'login') {
    return authed ? { name: 'home' } : true
  }
  return authed ? true : { name: 'login' }
})
