<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import {
  NDataTable, NCard, NButton, NSpace, NTag, NPopconfirm,
  NMessageProvider, useMessage,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()
const loading = ref(false)
const withdraws = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const statusMap: Record<number, { type: string; label: string }> = {
  0: { type: 'warning', label: '待审核' },
  1: { type: 'success', label: '已通过' },
  2: { type: 'error', label: '已拒绝' },
}

const columns: DataTableColumns<any> = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '商户', key: 'merchant_name', width: 120 },
  {
    title: '金额',
    key: 'amount',
    width: 100,
    render(row) { return `¥${(row.amount / 100).toFixed(2)}` },
  },
  { title: '提现方式', key: 'type', width: 80 },
  { title: '账号', key: 'account', width: 180 },
  {
    title: '状态',
    key: 'status',
    width: 80,
    render(row) {
      const s = statusMap[row.status] || { type: 'default', label: '未知' }
      return h(NTag, { type: s.type as any, size: 'small' }, { default: () => s.label })
    },
  },
  { title: '备注', key: 'remark', ellipsis: { tooltip: true }, width: 150 },
  { title: '创建时间', key: 'created_at', width: 170 },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    render(row) {
      if (row.status !== 0) return h('span', null, '-')
      return h(NSpace, null, () => [
        h(NPopconfirm, {
          onPositiveClick: () => handleApprove(row),
        }, {
          trigger: () => h(NButton, { size: 'small', type: 'success' }, '通过'),
          default: () => '确认通过该提现申请？',
        }),
        h(NPopconfirm, {
          onPositiveClick: () => handleReject(row),
          showIcon: false,
        }, {
          trigger: () => h(NButton, { size: 'small', type: 'error' }, '拒绝'),
          default: () => '确认拒绝该提现申请？',
        }),
      ])
    },
  },
]

async function fetchWithdraws() {
  loading.value = true
  try {
    const { data } = await api.get('/api/admin/withdraws', {
      params: { page: page.value, page_size: pageSize.value },
    })
    if (data.code === 0) {
      withdraws.value = data.data.list || data.data
      total.value = data.data.total || 0
    }
  } catch (e: any) {
    message.error(e.response?.data?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function handleApprove(row: any) {
  try {
    const { data } = await api.post(`/api/admin/withdraws/${row.id}/approve`)
    if (data.code === 0) {
      message.success('已通过')
      row.status = 1
    } else {
      message.error(data.message || '操作失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.message || '操作失败')
  }
}

async function handleReject(row: any) {
  try {
    const { data } = await api.post(`/api/admin/withdraws/${row.id}/reject`)
    if (data.code === 0) {
      message.success('已拒绝')
      row.status = 2
    } else {
      message.error(data.message || '操作失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.message || '操作失败')
  }
}

function handlePageChange(p: number) { page.value = p; fetchWithdraws() }
function handlePageSizeChange(ps: number) { pageSize.value = ps; page.value = 1; fetchWithdraws() }

onMounted(() => {
  fetchWithdraws()
})
</script>

<template>
  <NMessageProvider>
    <div style="padding:24px">
      <NCard title="提现管理">
        <NDataTable
          :columns="columns"
          :data="withdraws"
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
    </div>
  </NMessageProvider>
</template>
