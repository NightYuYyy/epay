<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NCard, NGrid, NGi, NStatistic, NDataTable } from 'naive-ui'
import { useMessage } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()

const balance = ref(0)
const todayOrderCount = ref(0)

const todayStats = ref<
  { type: string; count: number; amount: number }[]
>([])

const statsColumns = [
  { title: '支付类型', key: 'type', width: 120 },
  { title: '订单数', key: 'count', width: 100 },
  {
    title: '金额',
    key: 'amount',
    render(row: any) {
      return (row.amount / 100).toFixed(2)
    },
  },
]

async function fetchBalance() {
  try {
    const { data } = await api.get('/api/merchant/balance')
    if (data.code === 0) {
      balance.value = data.data.balance || 0
    }
  } catch (err: any) {
    message.error('获取余额失败')
  }
}

async function fetchDashboard() {
  try {
    const { data } = await api.get('/api/merchant/dashboard')
    if (data.code === 0) {
      todayOrderCount.value = data.data.orderCount || 0
      todayStats.value = data.data.stats || []
    }
  } catch (err: any) {
    message.error('获取仪表盘数据失败')
  }
}

onMounted(() => {
  fetchBalance()
  fetchDashboard()
})
</script>

<template>
  <div>
    <n-grid :cols="2" :x-gap="12" :y-gap="12" responsive="screen">
      <n-gi>
        <n-card>
          <n-statistic label="可提现余额" :value="(balance / 100).toFixed(2)">
            <template #prefix>¥</template>
          </n-statistic>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card>
          <n-statistic label="今日订单数" :value="todayOrderCount">
            <template #suffix>单</template>
          </n-statistic>
        </n-card>
      </n-gi>
    </n-grid>

    <n-card v-if="todayStats.length > 0" title="今日订单统计" style="margin-top: 16px;">
      <n-dataTable
        :columns="statsColumns"
        :data="todayStats"
        :bordered="false"
        :single-line="false"
        size="small"
      />
    </n-card>
  </div>
</template>
