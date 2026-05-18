<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  NForm, NFormItem, NInput, NButton, NCard, NInputNumber,
  NSpace, NMessageProvider, useMessage, NDivider,
} from 'naive-ui'
import api from '@/api/client'

const message = useMessage()
const loading = ref(false)
const saving = ref(false)

const form = ref({
  official_alipay_rate: 0,
  official_wxpay_rate: 0,
  default_platform_rate: 0,
  alipay_app_id: '',
  alipay_private_key: '',
  alipay_public_key: '',
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

function hydrateForm(remote: Record<string, string> | null | undefined) {
  if (!remote) return
  for (const [k, raw] of Object.entries(remote)) {
    if (!(k in form.value)) continue
    if (NUMERIC_FIELDS.has(k)) {
      const n = Number(raw)
      ;(form.value as any)[k] = Number.isFinite(n) ? n : 0
    } else {
      ;(form.value as any)[k] = raw ?? ''
    }
  }
}

onMounted(async () => {
  loading.value = true
  try {
    const { data } = await api.get('/api/admin/configs')
    if (data.code === 0 && data.data) {
      hydrateForm(data.data)
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '加载失败')
  } finally {
    loading.value = false
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
  <NMessageProvider>
    <div style="padding:24px;max-width:720px">
      <h2 style="margin-bottom:24px">系统配置</h2>
      <NSpin :show="loading">
        <NCard title="费率配置">
          <NForm :model="form" label-placement="left" label-width="160">
            <NFormItem label="支付宝官方费率(%)">
              <NInputNumber v-model:value="form.official_alipay_rate" :min="0" :max="100" :step="0.01" style="width:200px" />
            </NFormItem>
            <NFormItem label="微信官方费率(%)">
              <NInputNumber v-model:value="form.official_wxpay_rate" :min="0" :max="100" :step="0.01" style="width:200px" />
            </NFormItem>
            <NFormItem label="平台默认费率(%)">
              <NInputNumber v-model:value="form.default_platform_rate" :min="0" :max="100" :step="0.01" style="width:200px" />
            </NFormItem>
          </NForm>
        </NCard>

        <NDivider />

        <NCard title="支付宝配置">
          <NForm :model="form" label-placement="left" label-width="160">
            <NFormItem label="App ID">
              <NInput v-model:value="form.alipay_app_id" />
            </NFormItem>
            <NFormItem label="应用私钥">
              <NInput v-model:value="form.alipay_private_key" type="textarea" :autosize="{ minRows: 3, maxRows: 6 }" />
            </NFormItem>
            <NFormItem label="支付宝公钥">
              <NInput v-model:value="form.alipay_public_key" type="textarea" :autosize="{ minRows: 3, maxRows: 6 }" />
            </NFormItem>
          </NForm>
        </NCard>

        <NDivider />

        <NCard title="微信支付配置">
          <NForm :model="form" label-placement="left" label-width="160">
            <NFormItem label="App ID">
              <NInput v-model:value="form.wxpay_app_id" />
            </NFormItem>
            <NFormItem label="商户号">
              <NInput v-model:value="form.wxpay_mch_id" />
            </NFormItem>
            <NFormItem label="API Key">
              <NInput v-model:value="form.wxpay_api_key" type="password" show-password-on="click" />
            </NFormItem>
            <NFormItem label="API V3 Key">
              <NInput v-model:value="form.wxpay_api_v3_key" type="password" show-password-on="click" />
            </NFormItem>
          </NForm>
        </NCard>

        <NSpace justify="end" style="margin-top:24px">
          <NButton type="primary" :loading="saving" @click="handleSave">保存配置</NButton>
        </NSpace>
      </NSpin>
    </div>
  </NMessageProvider>
</template>
