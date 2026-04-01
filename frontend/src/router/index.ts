import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    {
      name: 'notFound',
      path: '/:path(.*)+',
      redirect: {
        name: 'dashboard',
      },
    },
    {
      name: 'dashboard',
      path: '/dashboard',
      component: () => import('@/views/layout/index.vue'),
      redirect: { name: 'chat' },
      children: [
        {
          name: 'chat',
          path: 'chat',
          component: () => import('@/views/chat/index.vue'),
        },
        {
          name: 'model-configure',
          path: 'model',
          component: () => import('@/views/model/index.vue'),
        },
        {
          name: 'skill-configure',
          path: 'skill',
          component: () => import('@/views/skill/index.vue'),
        },
        {
          name: 'channel-configure',
          path: 'channel',
          component: () => import('@/views/channel/index.vue'),
        },
        {
          name: 'usage-dashboard',
          path: 'usage',
          component: () => import('@/views/usage/index.vue'),
        },
        {
          name: 'skill-audit',
          path: 'skill-audit',
          component: () => import('@/views/audit/index.vue'),
        },
        {
          name: 'storage-audit',
          path: 'storage-audit',
          component: () => import('@/views/audit/storage.vue'),
        },
        {
          name: 'request-log',
          path: 'request-log',
          component: () => import('@/views/audit/prompt.vue'),
        },
      ],
    },
  ],
})

export default router
