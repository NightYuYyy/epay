<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NForm, NFormItem, NInput, NButton, NCard, NMessageProvider, useMessage } from 'naive-ui'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()
const message = useMessage()

const form = ref({ username: '', password: '' })
const loading = ref(false)

async function handleLogin() {
  if (!form.value.username || !form.value.password) {
    message.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    await authStore.login(form.value.username, form.value.password, true)
    message.success('登录成功')
    router.push('/admin/dashboard')
  } catch (e: any) {
    message.error(e.response?.data?.message || '登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <NMessageProvider>
    <div style="display:flex;justify-content:center;align-items:center;min-height:100vh;background:var(--n-color-body)">
      <NCard title="Epay 管理后台" style="width:400px">
        <NForm :model="form">
          <NFormItem label="用户名">
            <NInput v-model:value="form.username" placeholder="请输入用户名" />
          </NFormItem>
          <NFormItem label="密码">
            <NInput v-model:value="form.password" type="password" placeholder="请输入密码" @keyup.enter="handleLogin" />
          </NFormItem>
          <NButton type="primary" block :loading="loading" @click="handleLogin">登录</NButton>
        </NForm>
      </NCard>
    </div>
  </NMessageProvider>
</template>
