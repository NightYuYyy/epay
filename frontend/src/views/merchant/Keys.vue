<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NInput, NButton, NSpace, useMessage } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()

const pid = ref<string>('')
const pkey = ref<string>('')
const notifyUrl = ref('')
const saving = ref(false)

async function fetchApiKey() {
  try {
    const { data } = await api.get('/api/merchant/api-key')
    if (data.code === 0) {
      pid.value = data.data.pid || ''
      pkey.value = data.data.pkey || ''
      notifyUrl.value = data.data.notify_url || ''
    } else {
      message.error(data.msg || '获取API密钥失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '获取API密钥失败')
  }
}

async function saveNotifyUrl() {
  saving.value = true
  try {
    const { data } = await api.put('/api/merchant/notify-url', {
      notify_url: notifyUrl.value,
    })
    if (data.code === 0) {
      message.success('保存成功')
    } else {
      message.error(data.msg || '保存失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '保存失败')
  } finally {
    saving.value = false
  }
}

function copyText(text: string, label: string) {
  navigator.clipboard.writeText(text).then(() => {
    message.success(`${label}已复制`)
  }).catch(() => {
    message.error('复制失败')
  })
}

onMounted(() => {
  fetchApiKey()
})
</script>

<template>
  <div>
    <n-card title="API 密钥" style="margin-bottom: 16px;">
      <div style="margin-bottom: 16px;">
        <div style="margin-bottom: 4px; font-size: 13px; color: var(--n-text-color-3);">商户PID</div>
        <div style="display: flex; align-items: center; gap: 8px;">
          <code style="flex: 1; font-size: 16px; font-weight: bold; padding: 8px 12px; background: var(--n-color-target); border-radius: 6px;">
            {{ pid || '---' }}
          </code>
          <n-button size="small" @click="copyText(pid, 'PID')">复制</n-button>
        </div>
      </div>

      <div>
        <div style="margin-bottom: 4px; font-size: 13px; color: var(--n-text-color-3);">商户密钥 (PKEY)</div>
        <div style="display: flex; align-items: center; gap: 8px;">
          <code style="flex: 1; font-size: 13px; padding: 8px 12px; background: var(--n-color-target); border-radius: 6px; word-break: break-all;">
            {{ pkey || '---' }}
          </code>
          <n-button size="small" @click="copyText(pkey, 'PKEY')">复制</n-button>
        </div>
      </div>
    </n-card>

    <n-card title="回调通知配置">
      <n-space vertical>
        <n-input
          v-model:value="notifyUrl"
          placeholder="请输入回调通知URL"
          style="max-width: 600px;"
        />
        <n-button type="primary" :loading="saving" @click="saveNotifyUrl">
          保存配置
        </n-button>
      </n-space>
    </n-card>
  </div>
</template>
