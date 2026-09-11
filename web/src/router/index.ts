import { createRouter, createWebHashHistory } from 'vue-router'
import ShellHost from '../shell/ShellHost.vue'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/boot' },
    { path: '/boot', component: ShellHost, props: { screen: 'Boot' } },
    { path: '/login', component: ShellHost, props: { screen: 'Login' } },
    { path: '/desktop', component: ShellHost, props: { screen: 'Desktop' } },
    { path: '/s/:token', component: () => import('../shell/SharePage.vue') },
    // 独立应用模式：#/app/<app> 全屏单应用（无桌面外壳；未登录先走极简登录）
    { path: '/app/:app', component: () => import('../shell/StandaloneApp.vue') },
    { path: '/:pathMatch(.*)*', redirect: '/boot' }
  ]
})

export default router
