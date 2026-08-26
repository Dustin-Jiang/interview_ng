import { createRouter, createWebHistory } from 'vue-router'
import { ensureAuthReady } from '@/composables/useAuth'

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
    },
    {
      // 根路由：欢迎界面（个人信息 + 快捷入口）。
      path: '/',
      name: 'home',
      component: () => import('@/views/WelcomeView.vue'),
    },
    {
      // 候选人查看页（左名册右详情，普通用户可见）。
      path: '/candidates',
      name: 'candidates',
      component: () => import('@/views/CandidateRecordsView.vue'),
    },
    {
      // 设置页：管理功能左右分栏整合（候选人管理 / 面试官 / 角色）。
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
          path: 'system-status',
          name: 'settings-system-status',
          component: () => import('@/views/SettingsSystemStatusView.vue'),
        },
      ],
    },
    {
      // 候场大屏：未完成名单 + 签到操作。
      path: '/waiting',
      name: 'waiting',
      component: () => import('@/views/WaitingBoardView.vue'),
    },
    {
      path: '/room/:roomId?',
      name: 'room',
      component: () => import('@/views/RoomView.vue'),
    },
    // 旧管理路径重定向到设置分区（保持书签/分享可用）。
    {
      path: '/candidates/manage',
      redirect: { name: 'settings-candidates' },
    },
    {
      path: '/users',
      redirect: { name: 'settings-users' },
    },
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
