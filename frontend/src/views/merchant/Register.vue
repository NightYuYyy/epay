<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NForm, NFormItem, NInput, NButton, NCard, NDivider, NModal } from 'naive-ui'
import { useMessage } from 'naive-ui'
import api from '@/api/client'

interface RegisterResult {
  pid: number
  pkey: string
}

const message = useMessage()
const router = useRouter()

const name = ref('')
const password = ref('')
const loading = ref(false)
const showResult = ref(false)
const result = ref<RegisterResult | null>(null)

async function handleRegister() {
  if (!name.value || !password.value) {
    message.error('请填写商户名称和密码')
    return
  }
  loading.value = true
  try {
    const { data } = await api.post('/api/merchant/register', {
      name: name.value,
      password: password.value,
    })
    if (data.code === 0) {
      result.value = data.data
      showResult.value = true
    } else {
      message.error(data.msg || '注册失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '注册失败，请检查网络')
  } finally {
    loading.value = false
  }
}

function handleOk() {
  showResult.value = false
  router.push('/merchant/login')
}
</script>

<template>
  <div style="display: flex; justify-content: center; align-items: center; min-height: calc(100vh - 48px);">
    <n-card title="商户注册" style="width: 400px; max-width: 90vw;">
      <n-form label-placement="left" label-width="80">
        <n-form-item label="商户名称">
          <n-input v-model:value="name" placeholder="请输入商户名称" />
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
        已有账号？<router-link to="/merchant/login">立即登录</router-link>
      </div>
    </n-card>

    <n-modal :show="showResult" title="注册成功" @update:show="showResult = false">
      <n-card v-if="result" style="width: 360px;">
        <div style="margin-bottom: 12px; font-size: 14px;">
          <strong>商户PID：</strong><code style="background: var(--n-color-target); padding: 2px 8px; border-radius: 4px;">{{ result.pid }}</code>
        </div>
        <div style="margin-bottom: 12px; font-size: 14px;">
          <strong>API 密钥（PKEY）：</strong><br />
          <code style="background: var(--n-color-target); padding: 4px 8px; border-radius: 4px; word-break: break-all; display: inline-block; margin-top: 4px;">{{ result.pkey }}</code>
        </div>
        <p style="color: var(--n-text-color-3); font-size: 13px; margin-top: 8px;">
          使用商户 PID 和注册密码登录商户中心；PKEY 是 EasyPay 接口签名密钥，仅显示一次，请妥善保管。
        </p>
        <n-button type="primary" block @click="handleOk">知道了</n-button>
      </n-card>
    </n-modal>
  </div>
</template>
