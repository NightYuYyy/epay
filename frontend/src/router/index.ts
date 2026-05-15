import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', redirect: '/admin/dashboard' },
  {
    path: '/admin',
    component: () => import('@/layouts/AdminLayout.vue'),
    children: [
      { path: 'login', name: 'AdminLogin', component: () => import('@/views/admin/Login.vue') },
      { path: 'dashboard', name: 'AdminDashboard', component: () => import('@/views/admin/Dashboard.vue') },
      { path: 'merchants', name: 'AdminMerchants', component: () => import('@/views/admin/Merchants.vue') },
      { path: 'orders', name: 'AdminOrders', component: () => import('@/views/admin/Orders.vue') },
      { path: 'withdraws', name: 'AdminWithdraws', component: () => import('@/views/admin/Withdraws.vue') },
      { path: 'config', name: 'AdminConfig', component: () => import('@/views/admin/Config.vue') },
    ],
  },
  {
    path: '/merchant',
    component: () => import('@/layouts/MerchantLayout.vue'),
    children: [
      { path: 'login', name: 'MerchantLogin', component: () => import('@/views/merchant/Login.vue') },
      { path: 'register', name: 'MerchantRegister', component: () => import('@/views/merchant/Register.vue') },
      { path: 'dashboard', name: 'MerchantDashboard', component: () => import('@/views/merchant/Dashboard.vue') },
      { path: 'orders', name: 'MerchantOrders', component: () => import('@/views/merchant/Orders.vue') },
      { path: 'withdraw', name: 'MerchantWithdraw', component: () => import('@/views/merchant/Withdraw.vue') },
      { path: 'keys', name: 'MerchantKeys', component: () => import('@/views/merchant/Keys.vue') },
    ],
  },
]

export default createRouter({ history: createWebHistory(), routes })
