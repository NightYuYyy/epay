<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()
const message = useMessage()

const email = ref('')
const password = ref('')
const loading = ref(false)

async function handleLogin() {
  if (!email.value || !password.value) {
    message.warning('请输入邮箱和密码')
    return
  }
  loading.value = true
  try {
    const res = await authStore.login(email.value, password.value, false)
    if (res.code === 0) {
      message.success('登录成功')
      router.push('/user/dashboard')
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
  <div class="login-shell">
    <div class="hero-panel">
      <div class="aurora-bg" />
      <div class="hero-content">
        <div class="brand-mark">
          <div class="brand-glyph">ε</div>
          <div>
            <div class="brand-name">Epay 用户中心</div>
            <div class="brand-tag">For users</div>
          </div>
        </div>

        <h1 class="hero-title">
          一处接入，<br />
          清算、订单、提现全打通
        </h1>
        <p class="hero-sub">
          使用邮箱与密码登录后即可管理产品、查询订单、申请提现。
        </p>

        <ul class="hero-points">
          <li><span class="check">✓</span> 实时订单与回调日志</li>
          <li><span class="check">✓</span> 自助提现与历史结算</li>
          <li><span class="check">✓</span> 可旋转 API 密钥与回调地址</li>
        </ul>
      </div>
    </div>

    <div class="form-panel">
      <div class="form-wrap">
        <h2 class="form-title">用户登录</h2>
        <p class="form-sub">使用注册邮箱与密码访问</p>

        <n-form label-placement="top" :show-feedback="false" style="margin-top: 28px">
          <n-form-item label="邮箱" style="margin-bottom: 16px">
            <n-input
              v-model:value="email"
              placeholder="例如 user@example.com"
              size="large"
              style="width: 100%"
            />
          </n-form-item>
          <n-form-item label="密码" style="margin-bottom: 24px">
            <n-input
              v-model:value="password"
              type="password"
              show-password-on="click"
              placeholder="••••••••"
              size="large"
              @keyup.enter="handleLogin"
            />
          </n-form-item>
          <n-button type="primary" block size="large" :loading="loading" @click="handleLogin">登录</n-button>
        </n-form>

        <div class="form-hint">
          还没有账号？
          <router-link to="/user/register">立即注册</router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-shell {
  display: grid;
  grid-template-columns: 1.05fr 1fr;
  min-height: 100vh;
  background: var(--color-canvas);
}
.hero-panel {
  position: relative;
  overflow: hidden;
  background: linear-gradient(160deg, #0a194d 0%, #1c1e54 55%, #2e2b8c 100%);
  color: #fff;
  padding: 60px 60px 80px;
  display: flex;
  flex-direction: column;
  justify-content: center;
}
.hero-content {
  position: relative;
  z-index: 1;
  max-width: 460px;
}
.brand-mark {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 60px;
}
.brand-glyph {
  width: 40px;
  height: 40px;
  border-radius: 12px;
  background: linear-gradient(135deg, #665efd 0%, #f96bee 100%);
  display: grid;
  place-items: center;
  font-weight: 700;
  font-size: 18px;
  letter-spacing: -0.02em;
}
.brand-name {
  font-family: var(--font-display);
  font-weight: 500;
  font-size: 18px;
}
.brand-tag {
  color: rgba(255, 255, 255, 0.6);
  font-size: 12px;
  margin-top: 2px;
}
.hero-title {
  font-family: var(--font-display);
  font-weight: 300;
  font-size: 48px;
  line-height: 1.1;
  letter-spacing: -0.03em;
  margin: 0 0 20px;
}
.hero-sub {
  color: rgba(255, 255, 255, 0.7);
  font-size: 16px;
  margin: 0 0 36px;
}
.hero-points {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
  color: rgba(255, 255, 255, 0.85);
  font-size: 14px;
}
.hero-points .check {
  display: inline-block;
  margin-right: 10px;
  color: #b9b9f9;
  font-weight: 600;
}
.form-panel {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
}
.form-wrap {
  width: 100%;
  max-width: 380px;
}
.form-title {
  font-family: var(--font-display);
  font-weight: 300;
  font-size: 30px;
  letter-spacing: -0.02em;
  color: var(--color-ink);
  margin: 0 0 8px;
}
.form-sub {
  color: var(--color-ink-mute);
  font-size: 14px;
  margin: 0;
}
.form-hint {
  margin-top: 18px;
  color: var(--color-ink-mute);
  font-size: 13px;
  text-align: center;
}
.form-hint a {
  color: var(--color-primary);
  text-decoration: none;
}
.form-hint a:hover {
  text-decoration: underline;
}
@media (max-width: 960px) {
  .login-shell {
    grid-template-columns: 1fr;
  }
  .hero-panel {
    display: none;
  }
}
</style>
