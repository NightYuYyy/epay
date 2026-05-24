<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
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
const canSubmit = computed(() => Boolean(
  amount.value && amount.value >= 0.01 && amount.value <= balance.value && accountInfo.value.trim() && !submitting.value,
))

const statusTagMap: Record<string, any> = {
  PENDING: { type: 'warning' as const, text: '待处理' },
  APPROVED: { type: 'info' as const, text: '已批准' },
  PAID: { type: 'success' as const, text: '已完成' },
  REJECTED: { type: 'error' as const, text: '已拒绝' },
}

const columns = [
  {
    title: '金额',
    key: 'amount',
    width: 120,
    render(row: any) {
      return `¥${row.amount.toFixed(2)}`
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
    const { data } = await api.get('/api/user/balance')
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
    const { data } = await api.get('/api/user/withdraws', {
      params: {
        page: pagination.value.page,
        limit: pagination.value.pageSize,
      },
    })
    if (data.code === 0) {
      withdraws.value = data.data?.items || []
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
  if (!canSubmit.value) {
    message.error(balance.value <= 0 ? '余额不足' : '请填写有效提现金额和收款信息')
    return
  }
  submitting.value = true
  try {
    const { data } = await api.post('/api/user/withdraws', {
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
            ¥{{ balance.toFixed(2) }}
          </span>
        </n-form-item>
        <p v-if="balance <= 0" style="margin: 0 0 12px 100px; color: var(--n-text-color-3); font-size: 13px;">
          余额不足，暂不能申请提现。
        </p>
        <n-form-item label="提现金额">
          <n-input-number
            v-model:value="amount"
            :min="0.01"
            :step="0.01"
            :precision="2"
            :max="balance"
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
      <n-button type="primary" :loading="submitting" :disabled="!canSubmit" @click="handleSubmit">
        提交提现申请
      </n-button>
    </n-card>

    <n-card title="提现记录">
      <n-data-table
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
