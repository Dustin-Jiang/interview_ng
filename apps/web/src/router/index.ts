import { createRouter, createWebHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
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
  ],
})
