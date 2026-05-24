<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import { useRouter } from 'vue-router'
import {
  NDataTable, NButton, NModal, NForm, NFormItem, NInput, NInputNumber,
  NSpace, NTag, useMessage, NSelect,
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import api from '@/api/client'

interface User {
  id: string
  email: string
  name: string
  fee_rate: number
  status: 'active' | 'disabled'
  created_at: string
  updated_at: string
}

const router = useRouter()
const message = useMessage()
const loading = ref(false)
const users = ref<User[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

const showModal = ref(false)
const isEdit = ref(false)
const modalLoading = ref(false)

const form = ref({
  id: '' as string,
  email: '',
  password: '',
  name: '',
  fee_rate: 0.01,
  status: 'active' as 'active' | 'disabled',
})

const statusOptions = [
  { label: '启用', value: 'active' },
  { label: '禁用', value: 'disabled' },
]

function fmtDate(s: string) {
  if (!s) return ''
  return s.replace('T', ' ').replace('Z', '').slice(0, 19)
}

const columns: DataTableColumns<User> = [
  { title: '邮箱', key: 'email', width: 200 },
  { title: '名称', key: 'name', width: 160 },
  {
    title: '费率',
    key: 'fee_rate',
    width: 110,
    render(row) {
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
          h(NButton, { size: 'small', secondary: true, onClick: () => router.push(`/admin/products?user_id=${row.id}`) }, { default: () => '产品' }),
        ],
      })
    },
  },
]

async function fetchUsers() {
  loading.value = true
  try {
    const { data } = await api.get('/api/admin/users', {
      params: { page: page.value, limit: pageSize.value },
    })
    if (data.code === 0) {
      users.value = data.data.items || []
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
    email: '',
    password: '',
    name: '',
    fee_rate: 0.01,
    status: 'active',
  }
}

function openCreate() {
  isEdit.value = false
  resetForm()
  showModal.value = true
}

function openEdit(row: User) {
  isEdit.value = true
  form.value = {
    id: row.id,
    email: row.email,
    password: '',
    name: row.name,
    fee_rate: row.fee_rate,
    status: row.status,
  }
  showModal.value = true
}

async function handleSubmit() {
  if (isEdit.value) {
    if (!form.value.name) {
      message.warning('名称不能为空')
      return
    }
  } else {
    if (!form.value.email || !form.value.password || !form.value.name) {
      message.warning('邮箱、密码、名称不能为空')
      return
    }
  }
  modalLoading.value = true
  try {
    let res
    if (isEdit.value) {
      res = await api.put(`/api/admin/users/${form.value.id}`, {
        name: form.value.name,
        fee_rate: form.value.fee_rate,
        status: form.value.status,
      })
    } else {
      res = await api.post('/api/admin/users', {
        email: form.value.email,
        password: form.value.password,
        name: form.value.name,
        fee_rate: form.value.fee_rate,
      })
    }
    if (res.data.code === 0) {
      message.success(isEdit.value ? '修改成功' : '创建成功')
      showModal.value = false
      fetchUsers()
    } else {
      message.error(res.data.msg || '操作失败')
    }
  } catch (e: any) {
    message.error(e.response?.data?.msg || '操作失败')
  } finally {
    modalLoading.value = false
  }
}

function handlePageChange(p: number) {
  page.value = p
  fetchUsers()
}

function handlePageSizeChange(ps: number) {
  pageSize.value = ps
  page.value = 1
  fetchUsers()
}

onMounted(() => {
  fetchUsers()
})
</script>

<template>
  <div class="page">
    <header class="page-head">
      <div>
        <h1 class="page-title">用户管理</h1>
        <p class="page-sub">管理平台已注册用户、费率与状态。</p>
      </div>
      <n-button type="primary" @click="openCreate">+ 新增用户</n-button>
    </header>

    <div class="data-card">
      <n-data-table
        :columns="columns"
        :data="users"
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
      :title="isEdit ? '编辑用户' : '新增用户'"
    >
      <n-form :model="form" label-placement="top" :show-feedback="false">
        <n-form-item v-if="!isEdit" label="邮箱" style="margin-bottom: 16px">
          <n-input v-model:value="form.email" placeholder="user@example.com" />
        </n-form-item>
        <n-form-item v-if="!isEdit" label="密码" style="margin-bottom: 16px">
          <n-input v-model:value="form.password" type="password" placeholder="至少6位" />
        </n-form-item>
        <n-form-item label="名称" style="margin-bottom: 16px">
          <n-input v-model:value="form.name" placeholder="例如：张三" />
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
</style>
