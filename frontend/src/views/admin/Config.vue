<script setup lang="ts">
import { ref, onMounted, shallowRef } from 'vue'
import {
  NForm, NFormItem, NInput, NButton, NInputNumber, NSwitch,
  useMessage,
} from 'naive-ui'
import api from '@/api/client'

const message = useMessage()
const saving = ref(false)
const loaded = shallowRef(false)

const form = ref({
  official_alipay_rate: 0,
  official_wxpay_rate: 0,
  default_platform_rate: 0,
  alipay_app_id: '',
  alipay_private_key: '',
  alipay_public_key: '',
  alipay_notify_url: '',
  alipay_return_url: '',
  alipay_production: false,
  wxpay_app_id: '',
  wxpay_mch_id: '',
  wxpay_api_key: '',
  wxpay_api_v3_key: '',
})

// Backend stores values as strings (rainbow-style key→value text store).
// Coerce known numeric fields back to numbers so n-input-number can render
// them — otherwise the inputs show the placeholder and the operator thinks
// no value is configured.
const NUMERIC_FIELDS = new Set([
  'official_alipay_rate',
  'official_wxpay_rate',
  'default_platform_rate',
])
const BOOLEAN_FIELDS = new Set([
  'alipay_production',
])

function hydrateForm(remote: Record<string, string> | null | undefined) {
  if (!remote) return
  for (const [k, raw] of Object.entries(remote)) {
    if (!(k in form.value)) continue
    if (NUMERIC_FIELDS.has(k)) {
      const n = Number(raw)
      ;(form.value as any)[k] = Number.isFinite(n) ? n : 0
    } else if (BOOLEAN_FIELDS.has(k)) {
      ;(form.value as any)[k] = raw === 'true'
    } else {
      ;(form.value as any)[k] = raw ?? ''
    }
  }
}

onMounted(async () => {
  try {
    const { data } = await api.get('/api/admin/configs')
    if (data.code === 0 && data.data) {
      hydrateForm(data.data)
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '加载失败')
  } finally {
    loaded.value = true
  }
})

async function handleSave() {
  saving.value = true
  try {
    const { data } = await api.put('/api/admin/configs', form.value)
    if (data.code === 0) {
      message.success('保存成功')
    } else {
      message.error(data.msg || '保存失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '保存失败')
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">系统配置</h1>
        <p class="page-sub">维护平台费率与支付通道密钥。变更立即生效。</p>
      </div>
      <n-button type="primary" :loading="saving" :disabled="!loaded" @click="handleSave">
        保存配置
      </n-button>
    </header>

    <!-- v-if 替代 NSpin：避免 leave 动画在路由切换时阻塞 -->
    <template v-if="loaded">
      <!-- 费率 -->
      <section class="card-section">
        <header class="section-head">
          <h3>费率配置</h3>
          <p>百分比单位为小数（0.006 = 0.6%）</p>
        </header>
        <n-form :model="form" label-placement="top" :show-feedback="false">
          <div class="grid-3">
            <n-form-item label="支付宝官方费率">
              <n-input-number
                v-model:value="form.official_alipay_rate"
                :min="0"
                :max="1"
                :step="0.001"
                :precision="4"
                style="width: 100%"
              />
            </n-form-item>
            <n-form-item label="微信官方费率">
              <n-input-number
                v-model:value="form.official_wxpay_rate"
                :min="0"
                :max="1"
                :step="0.001"
                :precision="4"
                style="width: 100%"
              />
            </n-form-item>
            <n-form-item label="平台默认费率">
              <n-input-number
                v-model:value="form.default_platform_rate"
                :min="0"
                :max="1"
                :step="0.001"
                :precision="4"
                style="width: 100%"
              />
            </n-form-item>
          </div>
        </n-form>
      </section>

      <!-- 支付宝 -->
      <section class="card-section">
        <header class="section-head">
          <h3>支付宝配置</h3>
          <p>商户应用的密钥用于签名支付宝官方接口请求。</p>
        </header>
        <n-form :model="form" label-placement="top" :show-feedback="false">
          <n-form-item label="App ID">
            <n-input v-model:value="form.alipay_app_id" placeholder="2021000xxxxxxxxx" />
          </n-form-item>
          <n-form-item label="应用私钥 (PKCS#1/PKCS#8 PEM 或纯 base64)">
            <n-input
              v-model:value="form.alipay_private_key"
              type="textarea"
              :rows="4"
              placeholder="MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQ..."
            />
          </n-form-item>
          <n-form-item label="支付宝公钥">
            <n-input
              v-model:value="form.alipay_public_key"
              type="textarea"
              :rows="4"
              placeholder="MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA..."
            />
          </n-form-item>
          <n-form-item label="异步通知地址">
            <n-input
              v-model:value="form.alipay_notify_url"
              placeholder="https://your-domain.com/api/alipay/notify"
            />
          </n-form-item>
          <n-form-item label="同步跳转地址">
            <n-input
              v-model:value="form.alipay_return_url"
              placeholder="https://your-domain.com/demo.html"
            />
          </n-form-item>
          <n-form-item label="生产模式">
            <n-switch v-model:value="form.alipay_production" />
            <span style="margin-left: 8px; color: #889096; font-size: 13px;">
              {{ form.alipay_production ? '生产环境 (openapi.alipay.com)' : '沙箱环境 (openapi.alipaydev.com)' }}
            </span>
          </n-form-item>
        </n-form>
      </section>

      <!-- 微信 -->
      <section class="card-section">
        <header class="section-head">
          <h3>微信支付配置</h3>
          <p>所有字段都是可选的，留空表示未启用微信通道。</p>
        </header>
        <n-form :model="form" label-placement="top" :show-feedback="false">
          <div class="grid-2">
            <n-form-item label="App ID">
              <n-input v-model:value="form.wxpay_app_id" placeholder="wxabcdef0123456789" />
            </n-form-item>
            <n-form-item label="商户号">
              <n-input v-model:value="form.wxpay_mch_id" placeholder="1234567890" />
            </n-form-item>
          </div>
          <div class="grid-2">
            <n-form-item label="API Key">
              <n-input
                v-model:value="form.wxpay_api_key"
                type="password"
                show-password-on="click"
              />
            </n-form-item>
            <n-form-item label="API V3 Key">
              <n-input
                v-model:value="form.wxpay_api_v3_key"
                type="password"
                show-password-on="click"
              />
            </n-form-item>
          </div>
        </n-form>
      </section>
    </template>

    <!-- 加载占位：轻量骨架避免大块 NSpin -->
    <div v-else class="skeleton">
      <div class="skeleton-bar" />
      <div class="skeleton-bar w-2/3" />
      <div class="skeleton-bar w-1/3" />
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 22px;
  max-width: 880px;
}
.page-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
}
.page-title {
  font-family: var(--font-display);
  font-weight: 300;
  font-size: 28px;
  letter-spacing: -0.02em;
  color: var(--color-ink);
  margin: 0 0 6px;
}
.page-sub {
  color: var(--color-ink-mute);
  font-size: 13.5px;
  margin: 0;
}

.card-section {
  background: var(--color-canvas);
  border: 1px solid var(--color-hairline);
  border-radius: 14px;
  padding: 20px 24px 8px;
}
.section-head {
  margin-bottom: 14px;
}
.section-head h3 {
  font-family: var(--font-display);
  font-weight: 400;
  font-size: 16px;
  letter-spacing: -0.01em;
  color: var(--color-ink);
  margin: 0 0 4px;
}
.section-head p {
  color: var(--color-ink-mute);
  font-size: 12.5px;
  margin: 0;
}

.grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}
.grid-3 {
  display: grid;
  grid-template-columns: 1fr 1fr 1fr;
  gap: 0 16px;
}
@media (max-width: 720px) {
  .grid-2, .grid-3 { grid-template-columns: 1fr; }
}

.skeleton {
  background: var(--color-canvas);
  border: 1px solid var(--color-hairline);
  border-radius: 14px;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.skeleton-bar {
  height: 12px;
  border-radius: 6px;
  background: linear-gradient(
    90deg,
    var(--color-canvas-soft) 0%,
    var(--color-hairline) 50%,
    var(--color-canvas-soft) 100%
  );
  background-size: 200% 100%;
  animation: shimmer 1.4s ease-in-out infinite;
}
.skeleton-bar.w-2\/3 { width: 66%; }
.skeleton-bar.w-1\/3 { width: 33%; }
@keyframes shimmer {
  0% { background-position: 200% 0; }
  100% { background-position: -200% 0; }
}
</style>
