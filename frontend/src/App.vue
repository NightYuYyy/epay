<script setup lang="ts">
import { NConfigProvider, NMessageProvider, NLoadingBarProvider, NDialogProvider, NNotificationProvider } from 'naive-ui'
import { stripeLightOverrides } from './theme'
</script>

<template>
  <n-config-provider :theme-overrides="stripeLightOverrides">
    <n-loading-bar-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <n-message-provider>
            <router-view />
          </n-message-provider>
        </n-notification-provider>
      </n-dialog-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>
