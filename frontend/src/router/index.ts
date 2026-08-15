import { createRouter, createWebHistory } from 'vue-router'
import Dashboard from '../views/Dashboard.vue'
import HubHA from '../views/HubHA.vue'
import Spokes from '../views/Spokes.vue'
import ManagedSpokes from '../views/ManagedSpokes.vue'
import Provisioning from '../views/Provisioning.vue'
import WitnessSLA from '../views/WitnessSLA.vue'
import ConfigEditor from '../views/ConfigEditor.vue'
import Login from '../views/Login.vue'
import UserManagement from '../views/UserManagement.vue'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: Login,
    meta: { title: '用户登录', requiresAuth: false },
  },
  {
    path: '/',
    name: 'Dashboard',
    component: Dashboard,
    meta: { title: '仪表盘概览', requiresAuth: true },
  },
  {
    path: '/ha',
    name: 'HubHA',
    component: HubHA,
    meta: { title: 'Hub HA 集群管理', requiresAuth: true },
  },
  {
    path: '/spokes',
    name: 'Spokes',
    component: Spokes,
    meta: { title: 'Spoke 客户端管理', requiresAuth: true },
  },
  {
    path: '/managed-spokes',
    name: 'ManagedSpokes',
    component: ManagedSpokes,
    meta: { title: 'Spoke 设备管理', requiresAuth: true },
  },
  {
    path: '/provisioning',
    name: 'Provisioning',
    component: Provisioning,
    meta: { title: 'Spoke 配置向导', requiresAuth: true },
  },
  {
    path: '/witness',
    name: 'WitnessSLA',
    component: WitnessSLA,
    meta: { title: 'Witness 仲裁与 SLA', requiresAuth: true },
  },
  {
    path: '/config',
    name: 'ConfigEditor',
    component: ConfigEditor,
    meta: { title: '接口与配置中心', requiresAuth: true },
  },
  {
    path: '/users',
    name: 'UserManagement',
    component: UserManagement,
    meta: { title: '用户与权限管理', requiresAuth: true, adminOnly: true },
  },
  {
    path: '/:pathMatch(.*)*',
    redirect: '/',
  },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Navigation Guard
router.beforeEach((to, _from, next) => {
  const token = localStorage.getItem('opennhrp_token')
  const userJson = localStorage.getItem('opennhrp_user')
  const user = userJson ? JSON.parse(userJson) : null

  // 1. If route requires auth and not logged in, redirect to /login
  if (to.meta.requiresAuth !== false && !token) {
    next({ path: '/login', query: { redirect: to.fullPath } })
    return
  }

  // 2. If logged in and visiting /login, redirect to home
  if (to.path === '/login' && token) {
    next({ path: '/' })
    return
  }

  // 3. If route is adminOnly and current user is not admin
  if (to.meta.adminOnly && user?.role !== 'admin') {
    next({ path: '/' })
    return
  }

  next()
})
