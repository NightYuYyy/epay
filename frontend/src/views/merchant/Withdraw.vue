<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import { NCard, NForm, NFormItem, NInputNumber, NInput, NButton, NDataTable, NTag } from 'naive-ui'
import { useMessage } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()

// ---- Withdraw form ----
const amount = ref<number | null>(null)
const accountInfo = ref('')
const submitting = ref(false)
const balance = ref(0)

// ---- Withdraw history ----
const withdraws = ref<any[]>([])
const loading = ref(false)
const pagination = ref({ page: 1, pageSize: 10, itemCount: 0 })

const statusTagMap: Record<string, any> = {
  pending: { type: 'warning' as const, text: '待处理' },
  completed: { type: 'success' as const, text: '已完成' },
  rejected: { type: 'error' as const, text: '已拒绝' },
}

const columns = [
  {
    title: '金额',
    key: 'amount',
    width: 120,
    render(row: any) {
      return `¥${(row.amount / 100).toFixed(2)}`
    },
  },
  { title: '收款信息', key: 'account_info', width: 200, ellipsis: { tooltip: true } },
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
    title: '备注',
    key: 'remark',
    ellipsis: { tooltip: true },
  },
  {
    title: '申请时间',
    key: 'created_at',
    width: 170,
    render(row: any) {
      return row.created_at || '-'
    },
  },
]

async function fetchBalance() {
  try {
    const { data } = await api.get('/api/merchant/balance')
    if (data.code === 0) {
      balance.value = data.data.balance || 0
    }
  } catch {
    // silent
  }
}

async function fetchWithdraws() {
  loading.value = true
  try {
    const { data } = await api.get('/api/merchant/withdraws', {
      params: {
        page: pagination.value.page,
        page_size: pagination.value.pageSize,
      },
    })
    if (data.code === 0) {
      withdraws.value = data.data?.list || data.data || []
      pagination.value.itemCount = data.data?.total || 0
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '获取提现记录失败')
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.value.page = page
  fetchWithdraws()
}

function handlePageSizeChange(pageSize: number) {
  pagination.value.pageSize = pageSize
  pagination.value.page = 1
  fetchWithdraws()
}

async function handleSubmit() {
  if (!amount.value || amount.value <= 0) {
    message.error('请输入提现金额')
    return
  }
  if (!accountInfo.value) {
    message.error('请输入收款信息')
    return
  }
  submitting.value = true
  try {
    const { data } = await api.post('/api/merchant/withdraws', {
      amount: amount.value,
      account_info: accountInfo.value,
    })
    if (data.code === 0) {
      message.success('提现申请已提交')
      amount.value = null
      accountInfo.value = ''
      fetchBalance()
      pagination.value.page = 1
      fetchWithdraws()
    } else {
      message.error(data.msg || '提交失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '提交失败')
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  fetchBalance()
  fetchWithdraws()
})
</script>

<template>
  <div>
    <n-card title="申请提现" style="margin-bottom: 16px;">
      <n-form label-placement="left" label-width="100" style="max-width: 500px;">
        <n-form-item label="可提现余额">
          <span style="font-size: 18px; font-weight: bold; color: var(--n-color-target);">
            ¥{{ (balance / 100).toFixed(2) }}
          </span>
        </n-form-item>
        <n-form-item label="提现金额">
          <n-input-number
            v-model:value="amount"
            :min="0.01"
            :max="balance / 100"
            placeholder="请输入提现金额"
            style="width: 100%"
          >
            <template #prefix>¥</template>
          </n-input-number>
        </n-form-item>
        <n-form-item label="收款信息">
          <n-input
            v-model:value="accountInfo"
            type="textarea"
            placeholder="请输入支付宝账号或收款方式信息"
          />
        </n-form-item>
      </n-form>
      <n-button type="primary" :loading="submitting" @click="handleSubmit">
        提交提现申请
      </n-button>
    </n-card>

    <n-card title="提现记录">
      <n-dataTable
        :columns="columns"
        :data="withdraws"
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
  </div>
</template>
