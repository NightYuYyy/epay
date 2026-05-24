<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NForm, NFormItem, NInput, NButton, NCard, NDivider } from 'naive-ui'
import { useMessage } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()
const router = useRouter()

const email = ref('')
const password = ref('')
const name = ref('')
const loading = ref(false)

async function handleRegister() {
  if (!email.value || !password.value || !name.value) {
    message.warning('请填写邮箱、名称和密码')
    return
  }
  loading.value = true
  try {
    const { data } = await api.post('/api/user/register', {
      email: email.value,
      password: password.value,
      name: name.value,
    })
    if (data.code === 0) {
      message.success('注册成功，请登录')
      router.push('/user/login')
    } else {
      message.error(data.msg || '注册失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '注册失败，请检查网络')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div style="display: flex; justify-content: center; align-items: center; min-height: calc(100vh - 48px);">
    <n-card title="用户注册" style="width: 400px; max-width: 90vw;">
      <n-form label-placement="left" label-width="80">
        <n-form-item label="邮箱">
          <n-input v-model:value="email" placeholder="请输入邮箱地址" />
        </n-form-item>
        <n-form-item label="名称">
          <n-input v-model:value="name" placeholder="请输入用户名称" />
        </n-form-item>
        <n-form-item label="密码">
          <n-input
            v-model:value="password"
            type="password"
            show-password-on="click"
            placeholder="请输入密码"
            @keyup.enter="handleRegister"
          />
        </n-form-item>
      </n-form>
      <n-button
        type="primary"
        block
        :loading="loading"
        @click="handleRegister"
      >
        注册
      </n-button>
      <n-divider />
      <div style="text-align: center;">
        已有账号？<router-link to="/user/login">立即登录</router-link>
      </div>
    </n-card>
  </div>
</template>
