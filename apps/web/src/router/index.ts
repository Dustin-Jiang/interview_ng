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
      // 候选人管理页（管理员功能，对普通用户不可见）。
      path: '/candidates/manage',
      name: 'candidate-manage',
      component: () => import('@/views/CandidatesManageView.vue'),
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
    {
      path: '/users',
      name: 'users',
      component: () => import('@/views/UsersView.vue'),
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
