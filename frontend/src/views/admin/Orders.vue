<script setup lang="ts">
import { ref, onMounted, h, watch } from 'vue'
import {
  NDataTable, NSelect, NSpace, NTag, NButton, NInput, useMessage,
} from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import api from '@/api/client'

interface OrderRow {
  id: string
  order_no: string
  merchant_name: string
  type: string            // 'alipay' | 'wxpay'
  amount: number          // decimal yuan
  trade_no: string
  status: string          // PENDING/PAID/SETTLED/EXPIRED/CANCELLED
  notify_url: string
  paid_at: string | null
  created_at: string
}

const message = useMessage()
const loading = ref(false)
const orders = ref<OrderRow[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const merchantOptions = ref<SelectOption[]>([])
const statusFilter = ref<string | null>(null)
const merchantFilter = ref<string | null>(null)
const orderNoFilter = ref('')

const statusOptions = [
  { label: '全部', value: '' },
  { label: '待支付', value: 'PENDING' },
  { label: '已支付', value: 'PAID' },
  { label: '已结算', value: 'SETTLED' },
  { label: '已过期', value: 'EXPIRED' },
  { label: '已取消', value: 'CANCELLED' },
]

// Stripe-style tag palette — keeps the page calm; only paid/settled state
// gets a positive (green) tag, everything else stays neutral.
const statusMeta: Record<string, { type: 'success' | 'warning' | 'error' | 'default' | 'info'; label: string }> = {
  PENDING:   { type: 'warning', label: '待支付' },
  PAID:      { type: 'success', label: '已支付' },
  SETTLED:   { type: 'info', label: '已结算' },
  EXPIRED:   { type: 'default', label: '已过期' },
  CANCELLED: { type: 'error', label: '已取消' },
}

function fmtDate(s: string | null) {
  if (!s) return ''
  return s.replace('T', ' ').replace('Z', '').slice(0, 19)
}

const columns: DataTableColumns<OrderRow> = [
  { title: '订单号', key: 'order_no', width: 180, ellipsis: { tooltip: true } },
  { title: '商户', key: 'merchant_name', width: 160 },
  {
    title: '类型',
    key: 'type',
    width: 96,
    render(row) {
      const map: Record<string, string> = { alipay: '支付宝', wxpay: '微信' }
      return map[row.type] || row.type
    },
  },
  {
    title: '金额',
    key: 'amount',
    width: 120,
    render(row) {
      return h('span', { class: 'tabular' }, `¥ ${Number(row.amount).toFixed(2)}`)
    },
  },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render(row) {
      const s = statusMeta[row.status] || { type: 'default' as const, label: row.status }
      return h(NTag, { type: s.type, size: 'small', round: true }, { default: () => s.label })
    },
  },
  {
    title: '平台单号',
    key: 'trade_no',
    width: 200,
    ellipsis: { tooltip: true },
  },
  {
    title: '创建时间',
    key: 'created_at',
    width: 170,
    render(row) { return fmtDate(row.created_at) },
  },
]

async function fetchMerchants() {
  try {
    const { data } = await api.get('/api/admin/merchants', { params: { limit: 100 } })
    if (data.code === 0) {
      const list = data.data.items || []
      merchantOptions.value = [
        { label: '全部商户', value: '' },
        ...list.map((m: any) => ({ label: `${m.name} (#${m.pid})`, value: m.id })),
      ]
    }
  } catch { /* ignore */ }
}

async function fetchOrders() {
  loading.value = true
  try {
    const params: any = { page: page.value, limit: pageSize.value }
    if (statusFilter.value) params.status = statusFilter.value
    if (merchantFilter.value) params.merchant_id = merchantFilter.value
    if (orderNoFilter.value) params.order_no = orderNoFilter.value
    const { data } = await api.get('/api/admin/orders', { params })
    if (data.code === 0) {
      orders.value = data.data.items || []
      total.value = data.data.total || 0
    } else {
      message.error(data.msg || '加载失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '加载失败')
  } finally {
    loading.value = false
  }
}

function handlePageChange(p: number) {
  page.value = p
  fetchOrders()
}
function handlePageSizeChange(ps: number) {
  pageSize.value = ps
  page.value = 1
  fetchOrders()
}

function search() {
  page.value = 1
  fetchOrders()
}

watch([statusFilter, merchantFilter], () => {
  page.value = 1
  fetchOrders()
})

onMounted(async () => {
  await fetchMerchants()
  fetchOrders()
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">订单管理</h1>
        <p class="page-sub">查询全平台订单流水、支付状态与平台单号。</p>
      </div>
    </header>

    <div class="filter-card">
      <n-space :size="12" wrap>
        <n-input
          v-model:value="orderNoFilter"
          placeholder="订单号或外部订单号"
          clearable
          style="width: 220px"
          @keyup.enter="search"
        />
        <n-select
          v-model:value="merchantFilter"
          :options="merchantOptions"
          placeholder="选择商户"
          clearable
          style="width: 200px"
        />
        <n-select
          v-model:value="statusFilter"
          :options="statusOptions"
          placeholder="状态"
          clearable
          style="width: 140px"
        />
        <n-button type="primary" @click="search">查询</n-button>
      </n-space>
    </div>

    <div class="data-card">
      <n-data-table
        :columns="columns"
        :data="orders"
        :loading="loading"
        :pagination="{
          page,
          pageSize,
          itemCount: total,
          onChange: handlePageChange,
          onUpdatePageSize: handlePageSizeChange,
          showSizePicker: true,
          pageSizes: [10, 20, 50],
        }"
        :bordered="false"
      />
    </div>
  </div>
</template>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 22px;
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
.filter-card {
  background: var(--color-canvas);
  border: 1px solid var(--color-hairline);
  border-radius: 14px;
  padding: 16px 20px;
}
.data-card {
  background: var(--color-canvas);
  border: 1px solid var(--color-hairline);
  border-radius: 14px;
  padding: 8px 12px 14px;
}
:deep(.tabular) {
  font-variant-numeric: tabular-nums;
}
</style>
