<script setup lang="ts">
import { computed, h } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  NLayout,
  NLayoutSider,
  NLayoutHeader,
  NLayoutContent,
  NMenu,
  NIcon,
  NAvatar,
  NDropdown,
} from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()

function svgIcon(d: string) {
  return () =>
    h(NIcon, { size: 18 }, () =>
      h('svg', {
        viewBox: '0 0 24 24',
        fill: 'none',
        stroke: 'currentColor',
        'stroke-width': '1.6',
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
      }, [h('path', { d })]),
    )
}

const ICONS = {
  dashboard: svgIcon('M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z'),
  orders: svgIcon('M3 6h18M3 12h18M3 18h18'),
  withdraw: svgIcon('M12 2v20m6-12-6 6-6-6'),
  keys: svgIcon('M21 2l-9.6 9.6m0 0a4 4 0 1 1-5.66 5.66 4 4 0 0 1 5.66-5.66zM17 5l3 3M14 8l3 3'),
  logout: svgIcon('M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4m7 14 5-5-5-5m5 5H9'),
}

const menuOptions: MenuOption[] = [
  { label: '仪表盘', key: '/merchant/dashboard', icon: ICONS.dashboard },
  { label: '订单管理', key: '/merchant/orders', icon: ICONS.orders },
  { label: '提现', key: '/merchant/withdraw', icon: ICONS.withdraw },
  { label: 'API 密钥', key: '/merchant/keys', icon: ICONS.keys },
]

const activeKey = computed(() => route.path)
function handleMenuSelect(key: string) {
  router.push(key)
}

const userDropdownOptions = [
  { label: '退出登录', key: 'logout', icon: ICONS.logout },
]

function handleUserAction(key: string) {
  if (key === 'logout') {
    auth.logout()
    router.replace('/merchant/login')
  }
}

const username = computed(() => auth.user?.username || 'merchant')
const breadcrumb = computed(() => {
  const map: Record<string, string> = {
    '/merchant/dashboard': '仪表盘',
    '/merchant/orders': '订单管理',
    '/merchant/withdraw': '提现',
    '/merchant/keys': 'API 密钥',
  }
  return map[route.path] || ''
})

const sidebarMenuOverrides = {
  itemTextColor: 'rgba(255, 255, 255, 0.75)',
  itemTextColorHover: '#fff',
  itemTextColorActive: '#fff',
  itemTextColorActiveHover: '#fff',
  itemIconColor: 'rgba(255, 255, 255, 0.55)',
  itemIconColorHover: '#fff',
  itemIconColorActive: '#fff',
  itemIconColorActiveHover: '#fff',
  itemColorActive: 'rgba(255, 255, 255, 0.12)',
  itemColorActiveHover: 'rgba(255, 255, 255, 0.16)',
  arrowColor: 'rgba(255, 255, 255, 0.55)',
  arrowColorActive: '#fff',
  arrowColorHover: '#fff',
  arrowColorActiveHover: '#fff',
}
</script>

<template>
  <n-layout has-sider style="height: 100vh; background: var(--color-canvas-soft)">
    <n-layout-sider
      :native-scrollbar="false"
      :width="240"
      class="sidebar-stripe"
      :style="{ borderRight: 'none', position: 'relative' }"
    >
      <div style="padding: 26px 24px 22px; display: flex; align-items: center; gap: 10px">
        <div
          style="
            width: 34px;
            height: 34px;
            border-radius: 10px;
            background: linear-gradient(135deg, #665efd 0%, #f96bee 100%);
            display: grid;
            place-items: center;
            color: #fff;
            font-weight: 700;
            font-size: 16px;
            letter-spacing: -0.02em;
          "
        >ε</div>
        <div style="display: flex; flex-direction: column; line-height: 1.1">
          <span style="font-family: var(--font-display); font-weight: 500; font-size: 16px; color: #fff; letter-spacing: -0.01em">Epay</span>
          <span style="color: rgba(255, 255, 255, 0.55); font-size: 11px">Merchant portal</span>
        </div>
      </div>

      <div style="padding: 0 12px">
        <n-menu
          :value="activeKey"
          :options="menuOptions"
          :indent="18"
          @update:value="handleMenuSelect"
          :theme-overrides="sidebarMenuOverrides"
        />
      </div>
    </n-layout-sider>

    <n-layout style="background: var(--color-canvas-soft)">
      <n-layout-header
        bordered
        style="
          height: 60px;
          padding: 0 32px;
          display: flex;
          align-items: center;
          justify-content: space-between;
          background: var(--color-canvas);
        "
      >
        <div style="display: flex; align-items: center; gap: 10px">
          <span style="color: var(--color-ink-mute); font-size: 13px">商户中心</span>
          <span style="color: var(--color-hairline-strong); font-size: 12px">/</span>
          <span style="color: var(--color-ink); font-weight: 500">{{ breadcrumb }}</span>
        </div>
        <n-dropdown
          :options="userDropdownOptions"
          @select="handleUserAction"
          placement="bottom-end"
        >
          <div style="display: flex; align-items: center; gap: 10px; cursor: pointer">
            <n-avatar
              round
              :size="32"
              :style="{ background: 'var(--color-primary)', color: '#fff', fontSize: '12px', fontWeight: 600 }"
            >M</n-avatar>
            <div style="display: flex; flex-direction: column; line-height: 1.1">
              <span style="font-size: 13px; color: var(--color-ink); font-weight: 500">{{ username }}</span>
              <span style="font-size: 11px; color: var(--color-ink-mute)">商户</span>
            </div>
          </div>
        </n-dropdown>
      </n-layout-header>

      <n-layout-content style="padding: 28px 32px; background: var(--color-canvas-soft)">
        <router-view v-slot="{ Component, route }">
          <transition name="fade" mode="out-in">
            <component :is="Component" :key="route.fullPath" />
          </transition>
        </router-view>
      </n-layout-content>
    </n-layout>
  </n-layout>
</template>
