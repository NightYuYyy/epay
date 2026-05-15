<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  NConfigProvider,
  darkTheme,
  useOsTheme,
  NLayout,
  NLayoutSider,
  NLayoutHeader,
  NLayoutContent,
  NMenu,
  NButton,
  NH3,
} from 'naive-ui'
import type { MenuOption } from 'naive-ui'

const router = useRouter()
const route = useRoute()
const osThemeRef = useOsTheme()

const darkMode = ref(osThemeRef.value === 'dark')
const collapsed = ref(false)

const menuOptions: MenuOption[] = [
  { label: '仪表盘', key: '/admin/dashboard' },
  { label: '商户管理', key: '/admin/merchants' },
  { label: '订单管理', key: '/admin/orders' },
  { label: '提现管理', key: '/admin/withdraws' },
  { label: '系统配置', key: '/admin/config' },
]

const activeKey = computed(() => route.path)

function handleMenuSelect(key: string) {
  router.push(key)
}

function toggleDark() {
  darkMode.value = !darkMode.value
}
</script>

<template>
  <n-config-provider :theme="darkMode ? darkTheme : null">
    <n-layout has-sider style="height: 100vh">
      <n-layout-sider
        bordered
        :collapsed="collapsed"
        collapse-mode="width"
        :collapsed-width="64"
        :width="220"
        show-trigger="bar"
        @collapse="collapsed = true"
        @expand="collapsed = false"
      >
        <div style="height: 60px; display: flex; align-items: center; justify-content: center; font-weight: bold; font-size: 16px;">
          <span v-if="!collapsed">Epay 管理后台</span>
          <span v-else>EP</span>
        </div>
        <n-menu
          :value="activeKey"
          :options="menuOptions"
          :collapsed="collapsed"
          :collapsed-width="64"
          :collapsed-icon-size="22"
          @update:value="handleMenuSelect"
        />
        <div style="position: absolute; bottom: 16px; left: 0; right: 0; display: flex; justify-content: center;">
          <n-button quaternary @click="toggleDark" style="font-size: 18px;">
            {{ darkMode ? '☀️' : '🌙' }}
          </n-button>
        </div>
      </n-layout-sider>
      <n-layout>
        <n-layout-header bordered style="height: 60px; display: flex; align-items: center; justify-content: space-between; padding: 0 24px;">
          <n-h3 style="margin: 0;">Epay 管理后台</n-h3>
          <n-button quaternary @click="toggleDark">
            {{ darkMode ? '☀️ 浅色' : '🌙 深色' }}
          </n-button>
        </n-layout-header>
        <n-layout-content style="padding: 24px; min-height: calc(100vh - 60px);">
          <router-view />
        </n-layout-content>
      </n-layout>
    </n-layout>
  </n-config-provider>
</template>
