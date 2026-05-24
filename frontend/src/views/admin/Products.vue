<script setup lang="ts">
import { ref, onMounted, h, watch } from 'vue'
import { useRoute } from 'vue-router'
import {
  NDataTable, NButton, NModal, NForm, NFormItem, NInput, NInputNumber,
  NSpace, NTag, useMessage, NSelect, useDialog, NCheckbox,
} from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import api from '@/api/client'

interface Product {
  id: string
  user_id: string
  pid: number
  name: string
  description: string
  notify_url: string
  return_url: string
  fee_rate: number | null
  status: 'active' | 'disabled'
  keytype: string
  created_at: string
  updated_at: string
}

const route = useRoute()
const message = useMessage()
const dialog = useDialog()
const loading = ref(false)
const products = ref<Product[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const userOptions = ref<SelectOption[]>([])
const statusFilter = ref<string | null>(null)
const userIdFilter = ref<string | null>(null)

const showModal = ref(false)
const showKeyModal = ref(false)
const isEdit = ref(false)
const modalLoading = ref(false)
const apiKeyResult = ref<{ pid: number; pkey: string } | null>(null)
const clearFee = ref(false)

const form = ref({
  id: '' as string,
  user_id: '' as string,
  name: '',
  description: '',
  notify_url: '',
  return_url: '',
  fee_rate: null as number | null,
  status: 'active' as 'active' | 'disabled',
})

const statusOptions = [
  { label: '全部', value: '' },
  { label: '启用', value: 'active' },
  { label: '禁用', value: 'disabled' },
]

const editStatusOptions = [
  { label: '启用', value: 'active' },
  { label: '禁用', value: 'disabled' },
]

function fmtDate(s: string) {
  if (!s) return ''
  return s.replace('T', ' ').replace('Z', '').slice(0, 19)
}

function truncate(s: string, n: number) {
  if (!s) return ''
  return s.length > n ? s.slice(0, n) + '…' : s
}

const columns: DataTableColumns<Product> = [
  { title: 'PID', key: 'pid', width: 80 },
  { title: '名称', key: 'name', width: 160 },
  {
    title: '用户',
    key: 'user_id',
    width: 100,
    render(row) { return truncate(row.user_id, 8) },
  },
  {
    title: '通知地址',
    key: 'notify_url',
    ellipsis: { tooltip: true },
  },
  {
    title: '费率',
    key: 'fee_rate',
    width: 110,
    render(row) {
      if (row.fee_rate === null || row.fee_rate === undefined) return '继承用户'
      return `${(row.fee_rate * 100).toFixed(2)} %`
    },
  },
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
    title: '创建时间',
    key: 'created_at',
    width: 170,
    render(row) { return fmtDate(row.created_at) },
  },
  {
    title: '操作',
    key: 'actions',
    width: 220,
    render(row) {
      return h(NSpace, { size: 'small' }, {
        default: () => [
          h(NButton, { size: 'small', secondary: true, onClick: () => openEdit(row) }, { default: () => '编辑' }),
          h(NButton, { size: 'small', secondary: true, onClick: () => confirmRegenerate(row) }, { default: () => '重置密钥' }),
        ],
      })
    },
  },
]

async function fetchUsers() {
  try {
    const { data } = await api.get('/api/admin/users', { params: { limit: 100 } })
    if (data.code === 0) {
      const list = data.data.items || []
      userOptions.value = list.map((u: any) => ({ label: `${u.name} (${u.email})`, value: u.id }))
    }
  } catch { /* ignore */ }
}

async function fetchProducts() {
  loading.value = true
  try {
    const params: any = { page: page.value, limit: pageSize.value }
    if (statusFilter.value) params.status = statusFilter.value
    if (userIdFilter.value) params.user_id = userIdFilter.value
    const { data } = await api.get('/api/admin/products', { params })
    if (data.code === 0) {
      products.value = data.data.items || []
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
    user_id: '',
    name: '',
    description: '',
    notify_url: '',
    return_url: '',
    fee_rate: null,
    status: 'active',
  }
  clearFee.value = false
}

function openCreate() {
  isEdit.value = false
  resetForm()
  showModal.value = true
}

function openEdit(row: Product) {
  isEdit.value = true
  clearFee.value = false
  form.value = {
    id: row.id,
    user_id: row.user_id,
    name: row.name,
    description: row.description || '',
    notify_url: row.notify_url || '',
    return_url: row.return_url || '',
    fee_rate: row.fee_rate ?? null,
    status: row.status,
  }
  showModal.value = true
}

async function handleSubmit() {
  if (!form.value.name) {
    message.warning('产品名称不能为空')
    return
  }
  if (!isEdit.value && !form.value.user_id) {
    message.warning('请选择用户')
    return
  }
  modalLoading.value = true
  try {
    let res
    if (isEdit.value) {
      const body: any = {
        name: form.value.name,
        description: form.value.description || undefined,
        notify_url: form.value.notify_url || undefined,
        return_url: form.value.return_url || undefined,
        status: form.value.status,
      }
      if (clearFee.value) {
        body.clear_fee = true
      } else if (form.value.fee_rate !== null) {
        body.fee_rate = form.value.fee_rate
      }
      res = await api.put(`/api/admin/products/${form.value.id}`, body)
    } else {
      res = await api.post('/api/admin/products', {
        user_id: form.value.user_id,
        name: form.value.name,
        description: form.value.description || undefined,
        notify_url: form.value.notify_url || undefined,
        return_url: form.value.return_url || undefined,
        fee_rate: form.value.fee_rate ?? undefined,
      })
    }
    if (res.data.code === 0) {
      message.success(isEdit.value ? '修改成功' : '创建成功')
      showModal.value = false
      if (!isEdit.value && res.data.data) {
        apiKeyResult.value = { pid: res.data.data.pid, pkey: res.data.data.pkey }
        showKeyModal.value = true
      }
      fetchProducts()
    } else {
      message.error(res.data.msg || '操作失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '操作失败')
  } finally {
    modalLoading.value = false
  }
}

function confirmRegenerate(row: Product) {
  dialog.warning({
    title: '重置密钥',
    content: `确认重置产品「${row.name}」(PID ${row.pid}) 的 API 密钥？旧密钥将立即失效。`,
    positiveText: '确定',
    negativeText: '取消',
    onPositiveClick: () => regenerateKey(row),
  })
}

async function regenerateKey(row: Product) {
  try {
    const { data } = await api.post(`/api/admin/products/${row.id}/regenerate-pkey`)
    if (data.code === 0) {
      apiKeyResult.value = { pid: row.pid, pkey: data.data.pkey }
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
  fetchProducts()
}

function handlePageSizeChange(ps: number) {
  pageSize.value = ps
  page.value = 1
  fetchProducts()
}

function search() {
  page.value = 1
  fetchProducts()
}

watch([statusFilter, userIdFilter], () => {
  page.value = 1
  fetchProducts()
})

onMounted(async () => {
  // Check for user_id query param
  const qUser = route.query.user_id
  if (qUser && typeof qUser === 'string') {
    userIdFilter.value = qUser
  }
  await fetchUsers()
  fetchProducts()
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">产品管理</h1>
        <p class="page-sub">管理用户下产品、费率、通知地址与 API 密钥。</p>
      </div>
      <n-button type="primary" @click="openCreate">+ 新增产品</n-button>
    </header>

    <div class="filter-card">
      <n-space :size="12" wrap>
        <n-select
          v-model:value="userIdFilter"
          :options="[{ label: '全部用户', value: '' }, ...userOptions]"
          placeholder="选择用户"
          clearable
          style="width: 200px"
        />
        <n-select
          v-model:value="statusFilter"
          :options="statusOptions"
          placeholder="状态"
          clearable
          style="width: 140px"
        />
        <n-button type="primary" @click="search">查询</n-button>
      </n-space>
    </div>

    <div class="data-card">
      <n-data-table
        :columns="columns"
        :data="products"
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
      style="max-width: 560px"
      :title="isEdit ? '编辑产品' : '新增产品'"
    >
      <n-form :model="form" label-placement="top" :show-feedback="false">
        <n-form-item v-if="!isEdit" label="所属用户" style="margin-bottom: 16px">
          <n-select v-model:value="form.user_id" :options="userOptions" placeholder="选择用户" filterable />
        </n-form-item>
        <n-form-item label="产品名称" style="margin-bottom: 16px">
          <n-input v-model:value="form.name" placeholder="例如：线上商城" />
        </n-form-item>
        <n-form-item label="描述" style="margin-bottom: 16px">
          <n-input v-model:value="form.description" placeholder="可选" />
        </n-form-item>
        <n-form-item label="通知地址" style="margin-bottom: 16px">
          <n-input v-model:value="form.notify_url" placeholder="https://shop.example.com/notify" />
        </n-form-item>
        <n-form-item label="同步跳转" style="margin-bottom: 16px">
          <n-input v-model:value="form.return_url" placeholder="https://shop.example.com/return" />
        </n-form-item>
        <n-form-item label="费率（小数，0.01 = 1%，留空继承用户）" style="margin-bottom: 16px">
          <n-input-number
            v-model:value="form.fee_rate"
            :min="0"
            :max="1"
            :step="0.001"
            :precision="4"
            style="width: 100%"
            placeholder="留空则继承用户费率"
          />
        </n-form-item>
        <n-form-item v-if="isEdit" style="margin-bottom: 8px">
          <n-checkbox v-model:checked="clearFee">清除产品费率（恢复继承用户）</n-checkbox>
        </n-form-item>
        <n-form-item v-if="isEdit" label="状态" style="margin-bottom: 16px">
          <n-select v-model:value="form.status" :options="editStatusOptions" />
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
          <div class="key-label">产品 PID</div>
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
.filter-card {
  background: var(--color-canvas);
  border: 1px solid var(--color-hairline);
  border-radius: 14px;
  padding: 16px 20px;
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
