<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import { NCard, NDataTable, NSelect, NTag } from 'naive-ui'
import { useMessage } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()

const orders = ref<any[]>([])
const loading = ref(false)
const pagination = ref({ page: 1, pageSize: 10, itemCount: 0 })
const statusFilter = ref('')

const statusOptions = [
  { label: '全部', value: '' },
  { label: '待支付', value: 'pending' },
  { label: '已支付', value: 'paid' },
  { label: '已关闭', value: 'closed' },
  { label: '已退款', value: 'refunded' },
]

const statusTagMap: Record<string, any> = {
  pending: { type: 'warning' as const, text: '待支付' },
  paid: { type: 'success' as const, text: '已支付' },
  closed: { type: 'default' as const, text: '已关闭' },
  refunded: { type: 'error' as const, text: '已退款' },
}

const columns = [
  { title: '订单号', key: 'order_no', width: 180, ellipsis: { tooltip: true } },
  { title: '支付类型', key: 'type', width: 100 },
  {
    title: '金额',
    key: 'amount',
    width: 100,
    render(row: any) {
      return `¥${(row.amount / 100).toFixed(2)}`
    },
  },
  {
    title: '平台费',
    key: 'fee_platform',
    width: 100,
    render(row: any) {
      return `¥${(row.fee_platform / 100).toFixed(2)}`
    },
  },
  {
    title: '净额',
    key: 'net_amount',
    width: 100,
    render(row: any) {
      return `¥${(row.net_amount / 100).toFixed(2)}`
    },
  },
  {
    title: '状态',
    key: 'status',
    width: 80,
    render(row: any) {
      const tag = statusTagMap[row.status] || { type: 'default', text: row.status }
      return h(NTag, { type: tag.type, size: 'small' }, { default: () => tag.text })
    },
  },
  {
    title: '创建时间',
    key: 'created_at',
    width: 170,
    render(row: any) {
      return row.created_at || '-'
    },
  },
]

async function fetchOrders() {
  loading.value = true
  try {
    const params: any = {
      page: pagination.value.page,
      page_size: pagination.value.pageSize,
    }
    if (statusFilter.value) {
      params.status = statusFilter.value
    }
    const { data } = await api.get('/api/merchant/orders', { params })
    if (data.code === 0) {
      orders.value = data.data?.list || data.data || []
      pagination.value.itemCount = data.data?.total || 0
    } else {
      message.error(data.msg || '获取订单列表失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '获取订单列表失败')
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.value.page = page
  fetchOrders()
}

function handlePageSizeChange(pageSize: number) {
  pagination.value.pageSize = pageSize
  pagination.value.page = 1
  fetchOrders()
}

function handleStatusChange(val: string) {
  statusFilter.value = val
  pagination.value.page = 1
  fetchOrders()
}

onMounted(() => {
  fetchOrders()
})
</script>

<template>
  <n-card title="订单管理">
    <template #header-extra>
      <n-select
        v-model:value="statusFilter"
        :options="statusOptions"
        placeholder="筛选状态"
        clearable
        style="width: 140px;"
        @update:value="handleStatusChange"
      />
    </template>
    <n-dataTable
      :columns="columns"
      :data="orders"
      :loading="loading"
      :pagination="{
        page: pagination.page,
        pageSize: pagination.pageSize,
        itemCount: pagination.itemCount,
        showSizePicker: true,
        pageSizes: [10, 20, 50],
        onChange: handlePageChange,
        onUpdatePageSize: handlePageSizeChange,
      }"
      :bordered="false"
      :single-line="false"
      size="small"
    />
  </n-card>
</template>
