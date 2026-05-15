<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { NGrid, NGi, NCard, NStatistic, NSpin, NMessageProvider, useMessage } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()
const loading = ref(true)
const stats = ref({ today_orders: 0, today_income: 0, pending_withdraws: 0 })

onMounted(async () => {
  try {
    const { data } = await api.get('/api/admin/dashboard')
    if (data.code === 0) {
      stats.value = data.data
    }
  } catch (e: any) {
    message.error(e.response?.data?.message || '加载失败')
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <NMessageProvider>
    <div style="padding:24px">
      <h2 style="margin-bottom:24px">管理仪表盘</h2>
      <NSpin :show="loading">
        <NGrid :cols="3" :x-gap="24">
          <NGi>
            <NCard>
              <NStatistic label="今日订单数" :value="stats.today_orders" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="今日收入" :value="stats.today_income">
                <template #prefix>¥</template>
              </NStatistic>
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="待审核提现" :value="stats.pending_withdraws" />
            </NCard>
          </NGi>
        </NGrid>
      </NSpin>
    </div>
  </NMessageProvider>
</template>
