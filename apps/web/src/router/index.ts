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
      path: '/',
      name: 'candidates',
      component: () => import('@/views/CandidatesView.vue'),
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

// 全局守卫：未登录访问非 /login → 跳登录；已登录访问 /login → 跳候选人管理。
router.beforeEach(async (to) => {
  const authed = await ensureAuthReady()
  if (to.name === 'login') {
    return authed ? { name: 'candidates' } : true
  }
  return authed ? true : { name: 'login' }
})
