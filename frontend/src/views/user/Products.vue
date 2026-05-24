<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import {
  NCard,
  NDataTable,
  NButton,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NTag,
  NPopconfirm,
  NSpace,
  useMessage,
} from 'naive-ui'
import api from '@/api/client'

const message = useMessage()

const products = ref<any[]>([])
const loading = ref(false)
const pagination = ref({ page: 1, pageSize: 10, itemCount: 0 })

// Create modal
const showCreate = ref(false)
const createForm = ref({ name: '', description: '', notify_url: '', return_url: '', fee_rate: null as number | null })
const creating = ref(false)

// Edit modal
const showEdit = ref(false)
const editForm = ref({ id: '', name: '', description: '', notify_url: '', return_url: '', fee_rate: null as number | null })
const editing = ref(false)

// Pkey reveal modal
const showPkey = ref(false)
const pkeyValue = ref('')

// Create result modal shows the initial pkey after creation.
const showCreateResult = ref(false)
const createResult = ref<{ pid: string; pkey: string } | null>(null)

const columns = [
  { title: 'PID', key: 'pid', width: 120 },
  { title: '名称', key: 'name', width: 140, ellipsis: { tooltip: true } },
  { title: '回调地址', key: 'notify_url', width: 160, ellipsis: { tooltip: true } },
  { title: '跳转地址', key: 'return_url', width: 160, ellipsis: { tooltip: true } },
  {
    title: '费率',
    key: 'fee_rate',
    width: 80,
    render(row: any) {
      if (row.fee_rate === null || row.fee_rate === undefined) return '继承'
      return `${(row.fee_rate * 100).toFixed(2)}%`
    },
  },
  {
    title: '状态',
    key: 'status',
    width: 80,
    render(row: any) {
      const map: Record<string, any> = {
        active: { type: 'success' as const, text: '启用' },
        disabled: { type: 'default' as const, text: '禁用' },
      }
      const tag = map[row.status] || { type: 'default', text: row.status }
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
  {
    title: '操作',
    key: 'actions',
    width: 280,
    render(row: any) {
      return h(NSpace, { size: 'small' }, () => [
        h(
          NButton,
          { size: 'small', onClick: () => openEdit(row) },
          { default: () => '编辑' },
        ),
        h(
          NButton,
          { size: 'small', onClick: () => revealPkey(row.id) },
          { default: () => '查看密钥' },
        ),
        h(
          NPopconfirm,
          { onPositiveClick: () => regeneratePkey(row.id) },
          {
            default: () => '确定重新生成密钥？旧密钥将立即失效。',
            trigger: () => h(
              NButton,
              { size: 'small' },
              { default: () => '重置密钥' },
            ),
          },
        ),
        h(
          NButton,
          { size: 'small', onClick: () => openDemo(row.pid) },
          { default: () => 'Demo' },
        ),
      ])
    },
  },
]

async function fetchProducts() {
  loading.value = true
  try {
    const { data } = await api.get('/api/user/products', {
      params: {
        page: pagination.value.page,
        limit: pagination.value.pageSize,
      },
    })
    if (data.code === 0) {
      products.value = data.data?.items || []
      pagination.value.itemCount = data.data?.total || 0
    } else {
      message.error(data.msg || '获取产品列表失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '获取产品列表失败')
  } finally {
    loading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.value.page = page
  fetchProducts()
}

function handlePageSizeChange(pageSize: number) {
  pagination.value.pageSize = pageSize
  pagination.value.page = 1
  fetchProducts()
}

// ---- Create ----
function openCreate() {
  createForm.value = { name: '', description: '', notify_url: '', return_url: '', fee_rate: null }
  showCreate.value = true
}

async function handleCreate() {
  if (!createForm.value.name) {
    message.warning('请输入产品名称')
    return
  }
  creating.value = true
  try {
    const payload: any = {
      name: createForm.value.name,
      description: createForm.value.description,
      notify_url: createForm.value.notify_url,
      return_url: createForm.value.return_url,
    }
    if (createForm.value.fee_rate !== null && createForm.value.fee_rate !== undefined) {
      payload.fee_rate = createForm.value.fee_rate
    }
    const { data } = await api.post('/api/user/products', payload)
    if (data.code === 0) {
      showCreate.value = false
      createResult.value = { pid: data.data.pid, pkey: data.data.pkey }
      showCreateResult.value = true
      pagination.value.page = 1
      fetchProducts()
    } else {
      message.error(data.msg || '创建失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '创建失败')
  } finally {
    creating.value = false
  }
}

// ---- Edit ----
function openEdit(row: any) {
  editForm.value = {
    id: row.id,
    name: row.name || '',
    description: row.description || '',
    notify_url: row.notify_url || '',
    return_url: row.return_url || '',
    fee_rate: row.fee_rate,
  }
  showEdit.value = true
}

async function handleEdit() {
  editing.value = true
  try {
    const payload: any = {
      name: editForm.value.name,
      description: editForm.value.description,
      notify_url: editForm.value.notify_url,
      return_url: editForm.value.return_url,
    }
    if (editForm.value.fee_rate !== null && editForm.value.fee_rate !== undefined) {
      payload.fee_rate = editForm.value.fee_rate
    } else {
      payload.clear_fee = true
    }
    const { data } = await api.put(`/api/user/products/${editForm.value.id}`, payload)
    if (data.code === 0) {
      message.success('更新成功')
      showEdit.value = false
      fetchProducts()
    } else {
      message.error(data.msg || '更新失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '更新失败')
  } finally {
    editing.value = false
  }
}

// ---- Reveal Pkey ----
async function revealPkey(id: string) {
  try {
    const { data } = await api.get(`/api/user/products/${id}/secret`)
    if (data.code === 0) {
      pkeyValue.value = data.data.pkey || ''
      showPkey.value = true
    } else {
      message.error(data.msg || '获取密钥失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '获取密钥失败')
  }
}

// ---- Regenerate Pkey ----
async function regeneratePkey(id: string) {
  try {
    const { data } = await api.post(`/api/user/products/${id}/regenerate-pkey`)
    if (data.code === 0) {
      pkeyValue.value = data.data.pkey || ''
      showPkey.value = true
    } else {
      message.error(data.msg || '重置密钥失败')
    }
  } catch (err: any) {
    message.error(err.response?.data?.msg || '重置密钥失败')
  }
}

// ---- Demo ----
function openDemo(pid: string) {
  window.open(`/demo.html?pid=${encodeURIComponent(pid)}`, '_blank')
}

function copyText(text: string, label: string) {
  navigator.clipboard.writeText(text).then(() => {
    message.success(`${label}已复制`)
  }).catch(() => {
    message.error('复制失败')
  })
}

onMounted(() => {
  fetchProducts()
})
</script>

<template>
  <div>
    <n-card title="产品管理">
      <template #header-extra>
        <n-button type="primary" size="small" @click="openCreate">创建产品</n-button>
      </template>
      <n-data-table
        :columns="columns"
        :data="products"
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

    <!-- Create Modal -->
    <n-modal :show="showCreate" title="创建产品" @update:show="showCreate = false">
      <n-card style="width: 480px; max-width: 90vw;" role="dialog">
        <n-form label-placement="left" label-width="100">
          <n-form-item label="产品名称" required>
            <n-input v-model:value="createForm.name" placeholder="请输入产品名称" />
          </n-form-item>
          <n-form-item label="描述">
            <n-input v-model:value="createForm.description" placeholder="请输入产品描述" />
          </n-form-item>
          <n-form-item label="回调地址">
            <n-input v-model:value="createForm.notify_url" placeholder="支付回调通知URL" />
          </n-form-item>
          <n-form-item label="跳转地址">
            <n-input v-model:value="createForm.return_url" placeholder="支付完成后跳转URL" />
          </n-form-item>
          <n-form-item label="费率（可选）">
            <n-input-number
              v-model:value="createForm.fee_rate"
              :min="0"
              :max="1"
              :step="0.001"
              :precision="4"
              placeholder="留空继承默认费率；例如 0.006 = 0.60%；上限 100%"
              style="width: 100%"
            />
          </n-form-item>
        </n-form>
        <n-button type="primary" block :loading="creating" @click="handleCreate">创建</n-button>
      </n-card>
    </n-modal>

    <!-- Edit Modal -->
    <n-modal :show="showEdit" title="编辑产品" @update:show="showEdit = false">
      <n-card style="width: 480px; max-width: 90vw;" role="dialog">
        <n-form label-placement="left" label-width="100">
          <n-form-item label="产品名称" required>
            <n-input v-model:value="editForm.name" placeholder="请输入产品名称" />
          </n-form-item>
          <n-form-item label="描述">
            <n-input v-model:value="editForm.description" placeholder="请输入产品描述" />
          </n-form-item>
          <n-form-item label="回调地址">
            <n-input v-model:value="editForm.notify_url" placeholder="支付回调通知URL" />
          </n-form-item>
          <n-form-item label="跳转地址">
            <n-input v-model:value="editForm.return_url" placeholder="支付完成后跳转URL" />
          </n-form-item>
          <n-form-item label="费率（可选）">
            <n-input-number
              v-model:value="editForm.fee_rate"
              :min="0"
              :max="1"
              :step="0.001"
              :precision="4"
              placeholder="留空继承默认费率；例如 0.006 = 0.60%；上限 100%"
              style="width: 100%"
            />
          </n-form-item>
        </n-form>
        <n-button type="primary" block :loading="editing" @click="handleEdit">保存</n-button>
      </n-card>
    </n-modal>

    <!-- Pkey Modal -->
    <n-modal :show="showPkey" title="API 密钥" @update:show="showPkey = false">
      <n-card style="width: 480px; max-width: 90vw;" role="dialog">
        <div style="margin-bottom: 12px; font-size: 13px; color: var(--n-text-color-3);">PKEY</div>
        <div style="display: flex; align-items: center; gap: 8px;">
          <code style="flex: 1; font-size: 13px; padding: 8px 12px; background: var(--n-color-target); border-radius: 6px; word-break: break-all;">
            {{ pkeyValue }}
          </code>
          <n-button size="small" @click="copyText(pkeyValue, 'PKEY')">复制</n-button>
        </div>
        <p style="color: var(--n-text-color-3); font-size: 12px; margin-top: 10px;">
          请妥善保管，避免泄露；可在需要时再次查看或重置。
        </p>
      </n-card>
    </n-modal>

    <!-- Create Result Modal -->
    <n-modal :show="showCreateResult" title="产品创建成功" @update:show="showCreateResult = false">
      <n-card v-if="createResult" style="width: 480px; max-width: 90vw;" role="dialog">
        <div style="margin-bottom: 12px; font-size: 14px;">
          <strong>产品PID：</strong><code style="background: var(--n-color-target); padding: 2px 8px; border-radius: 4px;">{{ createResult.pid }}</code>
        </div>
        <div style="margin-bottom: 12px; font-size: 14px;">
          <strong>API 密钥（PKEY）：</strong><br />
          <code style="background: var(--n-color-target); padding: 4px 8px; border-radius: 4px; word-break: break-all; display: inline-block; margin-top: 4px;">{{ createResult.pkey }}</code>
        </div>
        <p style="color: var(--n-text-color-3); font-size: 13px; margin-top: 8px;">
          PKEY 是 EasyPay 接口签名密钥，请妥善保管，避免泄露。
        </p>
        <n-button type="primary" block @click="showCreateResult = false">知道了</n-button>
      </n-card>
    </n-modal>
  </div>
</template>
