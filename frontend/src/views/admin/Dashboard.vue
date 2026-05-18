<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { NSpin, useMessage } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()
const loading = ref(true)
const stats = ref<{ today_order_count?: number; today_revenue?: number; pending_withdraw_count?: number }>({})

const cards = computed(() => [
  {
    label: '今日订单',
    value: stats.value.today_order_count ?? 0,
    prefix: '',
    accent: 'linear-gradient(135deg, #665efd 0%, #533afd 100%)',
    delta: '实时',
  },
  {
    label: '今日收入',
    value: Number(stats.value.today_revenue ?? 0).toFixed(2),
    prefix: '¥ ',
    accent: 'linear-gradient(135deg, #f96bee 0%, #ea2261 100%)',
    delta: '今日订单累计金额',
  },
  {
    label: '待审核提现',
    value: stats.value.pending_withdraw_count ?? 0,
    prefix: '',
    accent: 'linear-gradient(135deg, #1ab87a 0%, #0a194d 100%)',
    delta: '待处理',
  },
])

onMounted(async () => {
  try {
    const { data } = await api.get('/api/admin/dashboard')
    if (data.code === 0) {
      stats.value = data.data
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '加载失败')
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">平台概览</h1>
        <p class="page-sub">实时聚合订单、收入与提现工作流的关键指标。</p>
      </div>
    </header>

    <n-spin :show="loading">
      <section class="stat-grid">
        <article v-for="c in cards" :key="c.label" class="stat-card">
          <div class="stat-accent" :style="{ background: c.accent }" />
          <div class="stat-label">{{ c.label }}</div>
          <div class="stat-value">
            <span class="stat-prefix">{{ c.prefix }}</span>{{ c.value }}
          </div>
          <div class="stat-delta">{{ c.delta }}</div>
        </article>
      </section>
    </n-spin>

    <section class="info-card">
      <div class="info-head">
        <h3>EasyPay 协议接入</h3>
        <span class="badge">v1 兼容彩虹</span>
      </div>
      <div class="info-grid">
        <div>
          <div class="info-key">创建订单</div>
          <code>POST /mapi.php</code>
        </div>
        <div>
          <div class="info-key">跳转支付</div>
          <code>GET / POST /submit.php</code>
        </div>
        <div>
          <div class="info-key">查询订单 (含 query / settle / order / orders / refund)</div>
          <code>GET /api.php?act=…</code>
        </div>
        <div>
          <div class="info-key">RSA s 路由（API_INIT）</div>
          <code>POST /api.php?s=pay/create</code>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 24px;
}
.page-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
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

.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 16px;
}
.stat-card {
  position: relative;
  background: var(--color-canvas);
  border: 1px solid var(--color-hairline);
  border-radius: 14px;
  padding: 24px 22px 20px;
  box-shadow: 0 1px 2px rgba(13, 37, 61, 0.03);
  overflow: hidden;
}
.stat-accent {
  position: absolute;
  inset: 0 0 auto 0;
  height: 3px;
}
.stat-label {
  color: var(--color-ink-mute);
  font-size: 12.5px;
  font-weight: 500;
  letter-spacing: 0.01em;
  text-transform: uppercase;
}
.stat-value {
  margin-top: 12px;
  font-family: var(--font-display);
  font-weight: 300;
  font-size: 36px;
  letter-spacing: -0.02em;
  color: var(--color-ink);
  font-variant-numeric: tabular-nums;
}
.stat-prefix {
  color: var(--color-ink-mute);
  font-size: 22px;
  margin-right: 4px;
}
.stat-delta {
  margin-top: 8px;
  color: var(--color-ink-mute);
  font-size: 12px;
}

.info-card {
  background: var(--color-canvas);
  border: 1px solid var(--color-hairline);
  border-radius: 14px;
  padding: 22px 24px;
}
.info-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.info-head h3 {
  font-family: var(--font-display);
  font-weight: 400;
  font-size: 17px;
  letter-spacing: -0.01em;
  color: var(--color-ink);
  margin: 0;
}
.badge {
  font-size: 11px;
  font-weight: 500;
  color: var(--color-primary);
  background: rgba(83, 58, 253, 0.08);
  padding: 3px 8px;
  border-radius: 999px;
}
.info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px 24px;
}
.info-grid > div {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.info-key {
  color: var(--color-ink-mute);
  font-size: 12px;
}
.info-grid code {
  font-family: var(--font-mono);
  font-size: 12.5px;
  color: var(--color-ink);
  background: var(--color-canvas-soft);
  border: 1px solid var(--color-hairline);
  border-radius: 6px;
  padding: 4px 8px;
  width: fit-content;
}
@media (max-width: 720px) {
  .info-grid {
    grid-template-columns: 1fr;
  }
}
</style>
