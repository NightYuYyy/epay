import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', name: 'Home', component: () => import('@/views/Home.vue') },

  // Standalone (no chrome) — login / register screens own the full viewport.
  { path: '/admin/login', name: 'AdminLogin', component: () => import('@/views/admin/Login.vue') },
  { path: '/user/login', name: 'UserLogin', component: () => import('@/views/user/Login.vue') },
  { path: '/user/register', name: 'UserRegister', component: () => import('@/views/user/Register.vue') },

  // Admin shell — sidebar + topbar layout.
  {
    path: '/admin',
    component: () => import('@/layouts/AdminLayout.vue'),
    children: [
      { path: 'dashboard', name: 'AdminDashboard', component: () => import('@/views/admin/Dashboard.vue') },
      { path: 'products', name: 'AdminProducts', component: () => import('@/views/admin/Products.vue') },
      { path: 'users', name: 'AdminUsers', component: () => import('@/views/admin/Users.vue') },
      { path: 'orders', name: 'AdminOrders', component: () => import('@/views/admin/Orders.vue') },
      { path: 'withdraws', name: 'AdminWithdraws', component: () => import('@/views/admin/Withdraws.vue') },
      { path: 'config', name: 'AdminConfig', component: () => import('@/views/admin/Config.vue') },
    ],
  },

  // User self-service shell — sidebar + topbar layout.
  {
    path: '/user',
    component: () => import('@/layouts/UserLayout.vue'),
    children: [
      { path: 'dashboard', name: 'UserDashboard', component: () => import('@/views/user/Dashboard.vue') },
      { path: 'products', name: 'UserProducts', component: () => import('@/views/user/Products.vue') },
      { path: 'orders', name: 'UserOrders', component: () => import('@/views/user/Orders.vue') },
      { path: 'withdraw', name: 'UserWithdraw', component: () => import('@/views/user/Withdraw.vue') },
    ],
  },
]

export default createRouter({ history: createWebHistory(), routes })
