import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', redirect: '/admin/dashboard' },

  // Standalone (no chrome) — login / register screens own the full viewport.
  { path: '/admin/login', name: 'AdminLogin', component: () => import('@/views/admin/Login.vue') },
  { path: '/merchant/login', name: 'MerchantLogin', component: () => import('@/views/merchant/Login.vue') },
  { path: '/merchant/register', name: 'MerchantRegister', component: () => import('@/views/merchant/Register.vue') },

  // Admin shell — sidebar + topbar layout.
  {
    path: '/admin',
    component: () => import('@/layouts/AdminLayout.vue'),
    children: [
      { path: 'dashboard', name: 'AdminDashboard', component: () => import('@/views/admin/Dashboard.vue') },
      { path: 'merchants', name: 'AdminMerchants', component: () => import('@/views/admin/Merchants.vue') },
      { path: 'orders', name: 'AdminOrders', component: () => import('@/views/admin/Orders.vue') },
      { path: 'withdraws', name: 'AdminWithdraws', component: () => import('@/views/admin/Withdraws.vue') },
      { path: 'config', name: 'AdminConfig', component: () => import('@/views/admin/Config.vue') },
    ],
  },

  // Merchant shell — sidebar + topbar layout.
  {
    path: '/merchant',
    component: () => import('@/layouts/MerchantLayout.vue'),
    children: [
      { path: 'dashboard', name: 'MerchantDashboard', component: () => import('@/views/merchant/Dashboard.vue') },
      { path: 'orders', name: 'MerchantOrders', component: () => import('@/views/merchant/Orders.vue') },
      { path: 'withdraw', name: 'MerchantWithdraw', component: () => import('@/views/merchant/Withdraw.vue') },
      { path: 'keys', name: 'MerchantKeys', component: () => import('@/views/merchant/Keys.vue') },
    ],
  },
]

export default createRouter({ history: createWebHistory(), routes })
