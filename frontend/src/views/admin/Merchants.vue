<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import {
  NDataTable, NButton, NModal, NForm, NFormItem, NInput, NInputNumber,
  NSpace, NTag, useMessage, NPopconfirm, NSelect,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import api from '@/api/client'

interface Merchant {
  id: string
  pid: number
  name: string
  fee_rate: number
  status: 'active' | 'disabled'
  notify_url: string
  created_at: string
  updated_at: string
}

const message = useMessage()
const loading = ref(false)
const merchants = ref<Merchant[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const showModal = ref(false)
const showKeyModal = ref(false)
const isEdit = ref(false)
const modalLoading = ref(false)
const apiKeyResult = ref<{ pid: number; pkey: string } | null>(null)

const form = ref({
  id: '' as string,
  name: '',
  fee_rate: 0.01,
  status: 'active' as 'active' | 'disabled',
  notify_url: '',
})

const statusOptions = [
  { label: '启用', value: 'active' },
  { label: '禁用', value: 'disabled' },
]

function fmtDate(s: string) {
  if (!s) return ''
  return s.replace('T', ' ').replace('Z', '').slice(0, 19)
}

const columns: DataTableColumns<Merchant> = [
  { title: 'PID', key: 'pid', width: 80 },
  { title: '商户名称', key: 'name' },
  {
    title: '状态',
    key: 'status',
    width: 90,
    render(row) {
      return row.status === 'active'
        ? h(NTag, { type: 'success', size: 'small', round: true }, { default: () => '启用' })
        : h(NTag, { type: 'error', size: 'small', round: true }, { default: () => '禁用' })
    },
  },
  {
    title: '费率',
    key: 'fee_rate',
    width: 110,
    render(row) {
      return `${(row.fee_rate * 100).toFixed(2)} %`
    },
  },
  {
    title: '通知地址',
    key: 'notify_url',
    ellipsis: { tooltip: true },
  },
  {
    title: '创建时间',
    key: 'created_at',
    width: 170,
    render(row) { return fmtDate(row.created_at) },
  },
  {
    title: '操作',
    key: 'actions',
    width: 260,
    render(row) {
      return h(NSpace, { size: 'small' }, {
        default: () => [
          h(NButton, { size: 'small', secondary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
          h(NButton, { size: 'small', secondary: true, onClick: () => regenerateKey(row) }, { default: () => '重置密钥' }),
          h(NPopconfirm, {
            onPositiveClick: () => toggleStatus(row),
          }, {
            trigger: () => h(NButton, {
              size: 'small',
              secondary: true,
              type: row.status === 'active' ? 'warning' : 'success',
            }, { default: () => row.status === 'active' ? '禁用' : '启用' }),
            default: () => `确认${row.status === 'active' ? '禁用' : '启用'}该商户？`,
          }),
        ],
      })
    },
  },
]

async function fetchMerchants() {
  loading.value = true
  try {
    const { data } = await api.get('/api/admin/merchants', {
      params: { page: page.value, limit: pageSize.value },
    })
    if (data.code === 0) {
      merchants.value = data.data.items || []
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

function resetForm() {
  form.value = {
    id: '',
    name: '',
    fee_rate: 0.01,
    status: 'active',
    notify_url: '',
  }
}

function openCreate() {
  isEdit.value = false
  resetForm()
  showModal.value = true
}

function openEdit(row: Merchant) {
  isEdit.value = true
  form.value = {
    id: row.id,
    name: row.name,
    fee_rate: row.fee_rate,
    status: row.status,
    notify_url: row.notify_url,
  }
  showModal.value = true
}

async function handleSubmit() {
  if (!form.value.name) {
    message.warning('商户名称不能为空')
    return
  }
  modalLoading.value = true
  try {
    let res
    if (isEdit.value) {
      res = await api.put(`/api/admin/merchants/${form.value.id}`, {
        name: form.value.name,
        fee_rate: form.value.fee_rate,
        status: form.value.status,
        notify_url: form.value.notify_url,
      })
    } else {
      res = await api.post('/api/admin/merchants', {
        name: form.value.name,
        fee_rate: form.value.fee_rate,
        notify_url: form.value.notify_url,
      })
    }
    if (res.data.code === 0) {
      message.success(isEdit.value ? '修改成功' : '创建成功')
      showModal.value = false
      // For create, surface the freshly assigned pid/pkey to the operator.
      if (!isEdit.value && res.data.data) {
        apiKeyResult.value = { pid: res.data.data.pid, pkey: res.data.data.pkey }
        showKeyModal.value = true
      }
      fetchMerchants()
    } else {
      message.error(res.data.msg || '操作失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '操作失败')
  } finally {
    modalLoading.value = false
  }
}

async function toggleStatus(row: Merchant) {
  try {
    const next: 'active' | 'disabled' = row.status === 'active' ? 'disabled' : 'active'
    const { data } = await api.put(`/api/admin/merchants/${row.id}`, { status: next })
    if (data.code === 0) {
      message.success('状态更新成功')
      row.status = next
    } else {
      message.error(data.msg || '操作失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '操作失败')
  }
}

async function regenerateKey(row: Merchant) {
  try {
    const { data } = await api.post(`/api/admin/merchants/${row.id}/regenerate-key`)
    if (data.code === 0) {
      apiKeyResult.value = { pid: row.pid, pkey: data.pkey || data.data?.pkey || '' }
      showKeyModal.value = true
    } else {
      message.error(data.msg || '获取失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '获取失败')
  }
}

function handlePageChange(p: number) {
  page.value = p
  fetchMerchants()
}

function handlePageSizeChange(ps: number) {
  pageSize.value = ps
  page.value = 1
  fetchMerchants()
}

onMounted(() => {
  fetchMerchants()
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">商户管理</h1>
        <p class="page-sub">管理平台已注册商户、费率以及通知地址。</p>
      </div>
      <n-button type="primary" @click="openCreate">+ 新增商户</n-button>
    </header>

    <div class="data-card">
      <n-data-table
        :columns="columns"
        :data="merchants"
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

    <!-- Create / Edit -->
    <n-modal
      v-model:show="showModal"
      :mask-closable="false"
      preset="card"
      style="max-width: 520px"
      :title="isEdit ? '编辑商户' : '新增商户'"
    >
      <n-form :model="form" label-placement="top" :show-feedback="false">
        <n-form-item label="商户名称" style="margin-bottom: 16px">
          <n-input v-model:value="form.name" placeholder="例如：示例商户" />
        </n-form-item>
        <n-form-item label="费率（小数，0.01 = 1%）" style="margin-bottom: 16px">
          <n-input-number
            v-model:value="form.fee_rate"
            :min="0"
            :max="1"
            :step="0.001"
            :precision="4"
            style="width: 100%"
          />
        </n-form-item>
        <n-form-item label="通知地址" style="margin-bottom: 16px">
          <n-input v-model:value="form.notify_url" placeholder="https://shop.example.com/notify" />
        </n-form-item>
        <n-form-item v-if="isEdit" label="状态" style="margin-bottom: 16px">
          <n-select v-model:value="form.status" :options="statusOptions" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showModal = false">取消</n-button>
          <n-button type="primary" :loading="modalLoading" @click="handleSubmit">
            {{ isEdit ? '保存' : '创建' }}
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- API key reveal -->
    <n-modal
      v-model:show="showKeyModal"
      preset="card"
      style="max-width: 500px"
      title="API 密钥"
    >
      <p class="key-warning">PKEY 仅本次显示，请立刻复制并妥善保管。</p>
      <div v-if="apiKeyResult" class="key-grid">
        <div>
          <div class="key-label">商户 PID</div>
          <code class="key-value">{{ apiKeyResult.pid }}</code>
        </div>
        <div>
          <div class="key-label">PKEY</div>
          <code class="key-value">{{ apiKeyResult.pkey }}</code>
        </div>
      </div>
      <template #footer>
        <n-space justify="end">
          <n-button type="primary" @click="showKeyModal = false">完成</n-button>
        </n-space>
      </template>
    </n-modal>
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
.data-card {
  background: var(--color-canvas);
  border: 1px solid var(--color-hairline);
  border-radius: 14px;
  padding: 8px 12px 14px;
}
.key-warning {
  margin: 0 0 14px;
  color: var(--color-warning);
  font-size: 12.5px;
}
.key-grid {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.key-label {
  font-size: 12px;
  color: var(--color-ink-mute);
  margin-bottom: 4px;
}
.key-value {
  display: inline-block;
  font-family: var(--font-mono);
  font-size: 13px;
  color: var(--color-ink);
  background: var(--color-canvas-soft);
  border: 1px solid var(--color-hairline);
  border-radius: 6px;
  padding: 6px 10px;
  word-break: break-all;
  user-select: all;
}
</style>
