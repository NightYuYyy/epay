<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NForm, NFormItem, NInput, NButton, NInputNumber, NDivider, NCard } from 'naive-ui'
import { useMessage } from 'naive-ui'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()
const message = useMessage()

const pid = ref<number | null>(null)
const password = ref('')
const loading = ref(false)

async function handleLogin() {
  if (!pid.value || !password.value) {
    message.error('请输入商户PID和密码')
    return
  }
  loading.value = true
  try {
    const res = await authStore.login(String(pid.value), password.value, false)
    if (res.code === 0) {
      router.push('/merchant/dashboard')
    } else {
      message.error(res.msg || '登录失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '登录失败，请检查网络')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div style="display: flex; justify-content: center; align-items: center; min-height: calc(100vh - 48px);">
    <n-card title="商户登录" style="width: 400px; max-width: 90vw;">
      <n-form label-placement="left" label-width="80">
        <n-form-item label="商户PID">
          <n-input-number
            v-model:value="pid"
            :min="1"
            :show-button="false"
            placeholder="请输入商户PID"
            style="width: 100%"
          />
        </n-form-item>
        <n-form-item label="密码">
          <n-input
            v-model:value="password"
            type="password"
            show-password-on="click"
            placeholder="请输入密码"
            @keyup.enter="handleLogin"
          />
        </n-form-item>
      </n-form>
      <n-button
        type="primary"
        block
        :loading="loading"
        @click="handleLogin"
      >
        登录
      </n-button>
      <n-divider />
      <div style="text-align: center;">
        还没有账号？<router-link to="/merchant/register">立即注册</router-link>
      </div>
    </n-card>
  </div>
</template>
