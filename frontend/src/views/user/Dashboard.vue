<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NGrid, NGi, NStatistic, NDataTable, useMessage } from 'naive-ui'
import api from '@/api/client'
const message = useMessage()

const balance = ref(0)
const profile = ref<any>(null)
const recentOrders = ref<any[]>([])

const orderColumns = [
  { title: '订单号', key: 'order_no', width: 180, ellipsis: { tooltip: true } },
  { title: '支付类型', key: 'type', width: 100 },
  {
    title: '金额',
    key: 'amount',
    width: 100,
    render(row: any) {
      return `¥${row.amount.toFixed(2)}`
    },
  },
  {
    title: '状态',
    key: 'status',
    width: 80,
    render(row: any) {
      const map: Record<string, string> = {
        PENDING: '待支付',
        PAID: '已支付',
        SETTLED: '已结算',
        EXPIRED: '已过期',
        CANCELLED: '已取消',
      }
      return map[row.status] || row.status
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

async function fetchProfile() {
  try {
    const { data } = await api.get('/api/user/profile')
    if (data.code === 0) {
      profile.value = data.data
    } else {
      message.error(data.msg || '获取用户信息失败')
    }
  } catch (err) {
    console.warn('fetch profile failed', err)
    message.error('获取用户信息失败')
  }
}

async function fetchBalance() {
  try {
    const { data } = await api.get('/api/user/balance')
    if (data.code === 0) {
      balance.value = data.data.balance || 0
    } else {
      message.error(data.msg || '获取余额失败')
    }
  } catch (err) {
    console.warn('fetch balance failed', err)
    message.error('获取余额失败')
  }
}

async function fetchRecentOrders() {
  try {
    const { data } = await api.get('/api/user/orders', { params: { limit: 5 } })
    if (data.code === 0) {
      recentOrders.value = data.data?.items || []
    } else {
      message.error(data.msg || '获取最近订单失败')
    }
  } catch (err) {
    console.warn('fetch recent orders failed', err)
    message.error('获取最近订单失败')
  }
}

onMounted(() => {
  fetchProfile()
  fetchBalance()
  fetchRecentOrders()
})
</script>

<template>
  <div>
    <n-grid :cols="2" :x-gap="12" :y-gap="12" responsive="screen">
      <n-gi>
        <n-card>
          <n-statistic label="可提现余额" :value="balance.toFixed(2)">
            <template #prefix>¥</template>
          </n-statistic>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card>
          <n-statistic label="用户名称" :value="profile?.name || '-'">
          </n-statistic>
        </n-card>
      </n-gi>
    </n-grid>

    <n-card v-if="recentOrders.length > 0" title="最近订单" style="margin-top: 16px;">
      <n-data-table
        :columns="orderColumns"
        :data="recentOrders"
        :bordered="false"
        :single-line="false"
        size="small"
      />
    </n-card>
  </div>
</template>
