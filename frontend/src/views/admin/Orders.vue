<script setup lang="ts">
import { ref, onMounted, h, watch } from 'vue'
import {
  NDataTable, NCard, NSelect, NSpace, NTag, NButton, NInput,
  NMessageProvider, useMessage,
} from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()
const loading = ref(false)
const orders = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const merchantOptions = ref<SelectOption[]>([])
const statusFilter = ref<number>(-1)
const merchantFilter = ref<number | null>(null)
const orderNoFilter = ref('')

const statusOptions = [
  { label: '全部', value: -1 },
  { label: '待支付', value: 0 },
  { label: '已支付', value: 1 },
  { label: '已关闭', value: 2 },
]

const statusMap: Record<number, { type: string; label: string }> = {
  0: { type: 'warning', label: '待支付' },
  1: { type: 'success', label: '已支付' },
  2: { type: 'default', label: '已关闭' },
}

const columns: DataTableColumns<any> = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '订单号', key: 'order_no', width: 180 },
  { title: '商户', key: 'merchant_name', width: 120 },
  {
    title: '金额',
    key: 'amount',
    width: 100,
    render(row) { return `¥${(row.amount / 100).toFixed(2)}` },
  },
  { title: '支付方式', key: 'pay_type', width: 80 },
  {
    title: '状态',
    key: 'status',
    width: 80,
    render(row) {
      const s = statusMap[row.status] || { type: 'default', label: '未知' }
      return h(NTag, { type: s.type as any, size: 'small' }, { default: () => s.label })
    },
  },
  { title: '创建时间', key: 'created_at', width: 170 },
]

const showDetail = ref(false)
const detailOrder = ref<any>(null)

async function fetchMerchants() {
  try {
    const { data } = await api.get('/api/admin/merchants', { params: { page_size: 999 } })
    if (data.code === 0) {
      const list = data.data.list || data.data
      merchantOptions.value = [{ label: '全部商户', value: null }, ...list.map((m: any) => ({ label: m.name, value: m.id }))]
    }
  } catch { /* ignore */ }
}

async function fetchOrders() {
  loading.value = true
  try {
    const params: any = { page: page.value, page_size: pageSize.value }
    if (statusFilter.value >= 0) params.status = statusFilter.value
    if (merchantFilter.value) params.merchant_id = merchantFilter.value
    if (orderNoFilter.value) params.order_no = orderNoFilter.value
    const { data } = await api.get('/api/admin/orders', { params })
    if (data.code === 0) {
      orders.value = data.data.list || data.data
      total.value = data.data.total || 0
    }
  } catch (e: any) {
    message.error(e.response?.data?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

function handlePageChange(p: number) { page.value = p; fetchOrders() }
function handlePageSizeChange(ps: number) { pageSize.value = ps; page.value = 1; fetchOrders() }

// function viewDetail(row: any) {
//   detailOrder.value = row
//   showDetail.value = true
// }

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
  <NMessageProvider>
    <div style="padding:24px">
      <NCard title="订单管理">
        <template #header-extra>
          <NSpace>
            <NInput
              v-model:value="orderNoFilter"
              placeholder="订单号"
              clearable
              style="width:180px"
              @keyup.enter="page=1;fetchOrders()"
            />
            <NSelect
              v-model:value="merchantFilter"
              :options="merchantOptions"
              placeholder="商户"
              clearable
              style="width:140px"
            />
            <NSelect
              v-model:value="statusFilter"
              :options="statusOptions"
              placeholder="状态"
              style="width:120px"
            />
            <NButton type="primary" @click="page=1;fetchOrders()">查询</NButton>
          </NSpace>
        </template>
        <NDataTable
          :columns="columns"
          :data="orders"
          :loading="loading"
          :pagination="{
            page, pageSize, itemCount: total,
            onChange: handlePageChange,
            onUpdatePageSize: handlePageSizeChange,
            showSizePicker: true,
            pageSizes: [10, 20, 50],
          }"
        />
      </NCard>

      <NModal v-model:show="showDetail" title="订单详情">
        <NCard style="width:500px" closable @close="showDetail = false">
          <NDescriptions v-if="detailOrder" :column="1" label-placement="left">
            <NDescriptionsItem label="订单号">{{ detailOrder.order_no }}</NDescriptionsItem>
            <NDescriptionsItem label="商户">{{ detailOrder.merchant_name }}</NDescriptionsItem>
            <NDescriptionsItem label="金额">¥{{ (detailOrder.amount / 100).toFixed(2) }}</NDescriptionsItem>
            <NDescriptionsItem label="支付方式">{{ detailOrder.pay_type }}</NDescriptionsItem>
            <NDescriptionsItem label="状态">
              <NTag :type="(statusMap[detailOrder.status]?.type as any) || 'default'" size="small">
                {{ statusMap[detailOrder.status]?.label || '未知' }}
              </NTag>
            </NDescriptionsItem>
            <NDescriptionsItem label="创建时间">{{ detailOrder.created_at }}</NDescriptionsItem>
          </NDescriptions>
        </NCard>
      </NModal>
    </div>
  </NMessageProvider>
</template>
