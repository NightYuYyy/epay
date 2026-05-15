<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import {
  NDataTable, NButton, NModal, NForm, NFormItem, NInput, NSwitch,
  NCard, NSpace, NTag, NMessageProvider, useMessage, NPopconfirm,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import api from '@/api/client'

const message = useMessage()
const loading = ref(false)
const merchants = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const showModal = ref(false)
const showKeyModal = ref(false)
const isEdit = ref(false)
const modalLoading = ref(false)
const apiKey = ref('')
const form = ref({
  id: null as number | null,
  name: '',
  username: '',
  password: '',
  status: 1,
  balance: 0,
  rate: 0,
})

const columns: DataTableColumns<any> = [
  { title: 'ID', key: 'id', width: 60 },
  { title: '商户名称', key: 'name' },
  { title: '用户名', key: 'username' },
  {
    title: '状态',
    key: 'status',
    width: 80,
    render(row) {
      return row.status === 1
        ? h(NTag, { type: 'success', size: 'small' }, { default: () => '启用' })
        : h(NTag, { type: 'error', size: 'small' }, { default: () => '禁用' })
    },
  },
  {
    title: '余额',
    key: 'balance',
    width: 120,
    render(row) { return `¥${(row.balance / 100).toFixed(2)}` },
  },
  {
    title: '费率(%)',
    key: 'rate',
    width: 80,
    render(row) { return row.rate.toFixed(2) },
  },
  { title: '创建时间', key: 'created_at', width: 170 },
  {
    title: '操作',
    key: 'actions',
    width: 280,
    render(row) {
      return h(NSpace, null, () => [
        h(NButton, { size: 'small', onClick: () => openEdit(row) }, '编辑'),
        h(NButton, { size: 'small', onClick: () => showApiKey(row) }, '密钥'),
        h(NPopconfirm, {
          onPositiveClick: () => toggleStatus(row),
        }, {
          trigger: () => h(NButton, { size: 'small', type: row.status === 1 ? 'warning' : 'success' }, row.status === 1 ? '禁用' : '启用'),
          default: () => `确认${row.status === 1 ? '禁用' : '启用'}该商户？`,
        }),
      ])
    },
  },
]

async function fetchMerchants() {
  loading.value = true
  try {
    const { data } = await api.get('/api/admin/merchants', {
      params: { page: page.value, page_size: pageSize.value },
    })
    if (data.code === 0) {
      merchants.value = data.data.list || data.data
      total.value = data.data.total || 0
    }
  } catch (e: any) {
    message.error(e.response?.data?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.value = { id: null, name: '', username: '', password: '', status: 1, balance: 0, rate: 0 }
}

function openCreate() {
  isEdit.value = false
  resetForm()
  showModal.value = true
}

function openEdit(row: any) {
  isEdit.value = true
  form.value = {
    id: row.id,
    name: row.name,
    username: row.username,
    password: '',
    status: row.status,
    balance: row.balance,
    rate: row.rate,
  }
  showModal.value = true
}

async function handleSubmit() {
  modalLoading.value = true
  try {
    const payload = { ...form.value }
    if (isEdit.value && !payload.password) delete (payload as any).password
    let res
    if (isEdit.value) {
      res = await api.put(`/api/admin/merchants/${form.value.id}`, payload)
    } else {
      res = await api.post('/api/admin/merchants', payload)
    }
    if (res.data.code === 0) {
      message.success(isEdit.value ? '修改成功' : '创建成功')
      showModal.value = false
      fetchMerchants()
    } else {
      message.error(res.data.message || '操作失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.message || '操作失败')
  } finally {
    modalLoading.value = false
  }
}

async function toggleStatus(row: any) {
  try {
    const newStatus = row.status === 1 ? 0 : 1
    const { data } = await api.put(`/api/admin/merchants/${row.id}`, { status: newStatus })
    if (data.code === 0) {
      message.success('状态更新成功')
      row.status = newStatus
    } else {
      message.error(data.message || '操作失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.message || '操作失败')
  }
}

async function showApiKey(row: any) {
  try {
    const { data } = await api.post(`/api/admin/merchants/${row.id}/key`)
    if (data.code === 0) {
      apiKey.value = data.data.key
      showKeyModal.value = true
    } else {
      message.error(data.message || '获取失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.message || '获取失败')
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
  <NMessageProvider>
    <div style="padding:24px">
      <NCard title="商户管理">
        <template #header-extra>
          <NButton type="primary" @click="openCreate">新增商户</NButton>
        </template>
        <NDataTable
          :columns="columns"
          :data="merchants"
          :loading="loading"
          :pagination="{
            page: page,
            pageSize: pageSize,
            itemCount: total,
            onChange: handlePageChange,
            onUpdatePageSize: handlePageSizeChange,
            showSizePicker: true,
            pageSizes: [10, 20, 50],
          }"
        />
      </NCard>

      <!-- Create/Edit Modal -->
      <NModal v-model:show="showModal" :title="isEdit ? '编辑商户' : '新增商户'">
        <NCard style="width:520px" closable @close="showModal = false">
          <NForm :model="form">
            <NFormItem label="商户名称"><NInput v-model:value="form.name" /></NFormItem>
            <NFormItem label="用户名"><NInput v-model:value="form.username" /></NFormItem>
            <NFormItem label="密码">
              <NInput v-model:value="form.password" type="password" :placeholder="isEdit ? '留空则不修改' : '请输入密码'" />
            </NFormItem>
            <NFormItem label="状态"><NSwitch v-model:value="form.status" :checked-value="1" :unchecked-value="0" /></NFormItem>
            <NFormItem label="余额(分)"><NInput :value="String(form.balance)" @update:value="(v: string) => form.balance = Number(v)" /></NFormItem>
            <NFormItem label="费率(%)"><NInput :value="String(form.rate)" @update:value="(v: string) => form.rate = Number(v)" /></NFormItem>
          </NForm>
          <template #footer>
            <NSpace justify="end">
              <NButton @click="showModal = false">取消</NButton>
              <NButton type="primary" :loading="modalLoading" @click="handleSubmit">确定</NButton>
            </NSpace>
          </template>
        </NCard>
      </NModal>

      <!-- API Key Modal -->
      <NModal v-model:show="showKeyModal" title="API 密钥">
        <NCard style="width:480px" closable @close="showKeyModal = false">
          <p style="word-break:break-all;padding:12px;background:var(--n-color-embedded);border-radius:4px">
            {{ apiKey }}
          </p>
        </NCard>
      </NModal>
    </div>
  </NMessageProvider>
</template>
