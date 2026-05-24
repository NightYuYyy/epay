<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import {
  NDataTable, NCard, NButton, NSpace, NTag,
  useMessage, useDialog,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const withdraws = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const statusMap: Record<string, { type: 'success' | 'warning' | 'error' | 'default' | 'info'; label: string }> = {
  PENDING: { type: 'warning', label: '待审核' },
  APPROVED: { type: 'info', label: '已通过' },
  PAID: { type: 'success', label: '已打款' },
  REJECTED: { type: 'error', label: '已拒绝' },
}

const columns: DataTableColumns<any> = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '用户', key: 'user_name', width: 120 },
  {
    title: '金额',
    key: 'amount',
    width: 100,
    render(row) { return `¥${Number(row.amount || 0).toFixed(2)}` },
  },
  { title: '收款信息', key: 'account_info', width: 180, ellipsis: { tooltip: true } },
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
      if (row.status !== 'PENDING') return h('span', null, '-')
      return h(NSpace, null, () => [
        h(NButton, { size: 'small', type: 'success', onClick: () => confirmApprove(row) }, { default: () => '通过' }),
        h(NButton, { size: 'small', type: 'error', onClick: () => confirmReject(row) }, { default: () => '拒绝' }),
      ])
    },
  },
]

function confirmApprove(row: any) {
  dialog.warning({
    title: '通过提现申请',
    content: `确认通过用户「${row.user_name || '-'}」的 ¥${Number(row.amount || 0).toFixed(2)} 提现申请？`,
    positiveText: '通过',
    negativeText: '取消',
    onPositiveClick: () => handleApprove(row),
  })
}

function confirmReject(row: any) {
  dialog.warning({
    title: '拒绝提现申请',
    content: `确认拒绝用户「${row.user_name || '-'}」的 ¥${Number(row.amount || 0).toFixed(2)} 提现申请？`,
    positiveText: '拒绝',
    negativeText: '取消',
    onPositiveClick: () => handleReject(row),
  })
}

async function fetchWithdraws() {
  loading.value = true
  try {
    const { data } = await api.get('/api/admin/withdraws', {
      params: { page: page.value, limit: pageSize.value },
    })
    if (data.code === 0) {
      withdraws.value = data.data.items || []
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

async function handleApprove(row: any) {
  try {
    const { data } = await api.post(`/api/admin/withdraws/${row.id}/approve`)
    if (data.code === 0) {
      message.success('已通过')
      row.status = 'APPROVED'
    } else {
      message.error(data.msg || '操作失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '操作失败')
  }
}

async function handleReject(row: any) {
  try {
    const { data } = await api.post(`/api/admin/withdraws/${row.id}/reject`, { remark: '管理员拒绝' })
    if (data.code === 0) {
      message.success('已拒绝')
      row.status = 'REJECTED'
    } else {
      message.error(data.msg || '操作失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '操作失败')
  }
}

function handlePageChange(p: number) { page.value = p; fetchWithdraws() }
function handlePageSizeChange(ps: number) { pageSize.value = ps; page.value = 1; fetchWithdraws() }

onMounted(() => {
  fetchWithdraws()
})
</script>

<template>
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
</template>
