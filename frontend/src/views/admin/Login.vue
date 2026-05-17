<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { NForm, NFormItem, NInput, NButton, useMessage } from 'naive-ui'
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
    const result = await authStore.login(form.value.username, form.value.password, true)
    if (result.code === 0) {
      message.success('登录成功')
      router.push('/admin/dashboard')
    } else {
      message.error(result.msg || '登录失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || e.message || '登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-shell">
    <!-- Hero panel — atmospheric gradient mesh (Stripe trademark) -->
    <div class="hero-panel">
      <div class="aurora-bg" />
      <div class="hero-content">
        <div class="brand-mark">
          <div class="brand-glyph">ε</div>
          <div>
            <div class="brand-name">Epay</div>
            <div class="brand-tag">Payment infrastructure</div>
          </div>
        </div>

        <h1 class="hero-title">
          一个商户接入<br />
          覆盖支付宝与微信
        </h1>
        <p class="hero-sub">
          兼容彩虹 EasyPay v1，全量 MD5 + RSA 双签，开箱即用的聚合支付平台。
        </p>

        <ul class="hero-points">
          <li><span class="check">✓</span> MD5 + RSA 双签，对接彩虹商户零改动</li>
          <li><span class="check">✓</span> GET 表单签名通知，重试节奏严格对齐</li>
          <li><span class="check">✓</span> 商户后台 / 平台后台 / 收银台一体</li>
        </ul>
      </div>
    </div>

    <!-- Login form panel -->
    <div class="form-panel">
      <div class="form-wrap">
        <h2 class="form-title">登录管理后台</h2>
        <p class="form-sub">使用平台分配的管理员凭据访问</p>

        <n-form :model="form" label-placement="top" :show-feedback="false" style="margin-top: 28px">
          <n-form-item label="用户名" style="margin-bottom: 16px">
            <n-input
              v-model:value="form.username"
              placeholder="admin"
              size="large"
            />
          </n-form-item>
          <n-form-item label="密码" style="margin-bottom: 24px">
            <n-input
              v-model:value="form.password"
              type="password"
              show-password-on="click"
              placeholder="••••••••"
              size="large"
              @keyup.enter="handleLogin"
            />
          </n-form-item>
          <n-button
            type="primary"
            block
            size="large"
            :loading="loading"
            @click="handleLogin"
          >
            登录
          </n-button>
        </n-form>

        <div class="form-hint">
          默认账号 <code>admin</code> / <code>admin123</code>，请登录后立即修改密码。
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

/* --- Hero panel ----------------------------------------------------- */
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
  letter-spacing: -0.01em;
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
  line-height: 1.55;
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

/* --- Form panel ----------------------------------------------------- */
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
  font-size: 12px;
  text-align: center;
}
.form-hint code {
  font-family: var(--font-mono);
  background: var(--color-canvas-soft);
  border: 1px solid var(--color-hairline);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  color: var(--color-ink);
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
