<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h2>用户与权限管理</h2>
        <span class="sub-title">管理系统登录账户，分配管理员与只读角色权限</span>
      </div>
      <n-button type="primary" @click="openCreateModal">
        + 添加新用户
      </n-button>
    </div>

    <!-- User Table Card -->
    <n-card class="user-card">
      <n-data-table
        :columns="columns"
        :data="users"
        :loading="loading"
        :bordered="false"
        :single-line="true"
        :scroll-x="600"
        :row-key="(row: UserRecord) => row.id"
      />
    </n-card>

    <!-- Create User Modal -->
    <n-modal
      v-model:show="showCreateModal"
      preset="card"
      title="创建新用户"
      style="width: 480px; max-width: calc(100vw - 32px);"
      :bordered="false"
    >
      <n-form ref="createFormRef" :model="createForm" :rules="createRules">
        <n-form-item label="用户名" path="username">
          <n-input v-model:value="createForm.username" placeholder="请输入唯一用户名" />
        </n-form-item>

        <n-form-item label="初始密码" path="password">
          <n-input
            v-model:value="createForm.password"
            type="password"
            show-password-on="click"
            placeholder="请输入初始密码（至少 6 位）"
          />
        </n-form-item>

        <n-form-item label="用户角色" path="role">
          <n-select
            v-model:value="createForm.role"
            :options="roleOptions"
            placeholder="请选择用户角色"
          />
        </n-form-item>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showCreateModal = false">取消</n-button>
          <n-button type="primary" :loading="submitting" @click="handleCreateUser">
            确认创建
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Edit User Modal -->
    <n-modal
      v-model:show="showEditModal"
      preset="card"
      :title="`编辑用户: ${editingUser?.username}`"
      style="width: 480px; max-width: calc(100vw - 32px);"
      :bordered="false"
    >
      <n-form ref="editFormRef" :model="editForm">
        <n-form-item label="用户名">
          <n-input :value="editingUser?.username" disabled />
        </n-form-item>

        <n-form-item label="用户角色" path="role">
          <n-select
            v-model:value="editForm.role"
            :options="roleOptions"
            placeholder="请选择用户角色"
          />
        </n-form-item>

        <n-form-item label="重置密码（留空则不修改）" path="password">
          <n-input
            v-model:value="editForm.password"
            type="password"
            show-password-on="click"
            placeholder="输入新密码（留空保持不变）"
          />
        </n-form-item>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showEditModal = false">取消</n-button>
          <n-button type="primary" :loading="submitting" @click="handleUpdateUser">
            保存修改
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import {
  NCard,
  NButton,
  NDataTable,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NSelect,
  NSpace,
  NTag,
  NIcon,
  NPopconfirm,
  createDiscreteApi,
  darkTheme,
  type DataTableColumns,
} from 'naive-ui'
import { EyeOutline, ShieldCheckmarkOutline } from '@vicons/ionicons5'
import { api } from '../api/client'
import { useAppStore } from '../store'
import type { UserRecord } from '../types'

const store = useAppStore()

const { message } = createDiscreteApi(['message'], {
  configProviderProps: computed(() => ({
    theme: store.isDark ? darkTheme : null,
  })),
})

const users = ref<UserRecord[]>([])
const loading = ref(false)
const submitting = ref(false)

const showCreateModal = ref(false)
const showEditModal = ref(false)
const editingUser = ref<UserRecord | null>(null)

const roleOptions = [
  { label: '管理员 (admin) - 完整读写与管理权限', value: 'admin' },
  { label: '只读用户 (readonly) - 仅可查看监控与日志', value: 'readonly' },
]

const createForm = ref({
  username: '',
  password: '',
  role: 'readonly' as 'admin' | 'readonly',
})

const editForm = ref({
  role: 'readonly' as 'admin' | 'readonly',
  password: '',
})

const createRules = {
  username: {
    required: true,
    message: '请输入用户名',
    trigger: 'blur',
  },
  password: {
    required: true,
    message: '请输入密码（至少 6 位）',
    trigger: 'blur',
  },
  role: {
    required: true,
    message: '请选择角色',
    trigger: 'change',
  },
}

const loadUsers = async () => {
  loading.value = true
  try {
    const list = await api.listUsers()
    users.value = list
  } catch (err: any) {
    message.error('加载用户列表失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

const openCreateModal = () => {
  createForm.value = {
    username: '',
    password: '',
    role: 'readonly',
  }
  showCreateModal.value = true
}

const handleCreateUser = async () => {
  if (!createForm.value.username || !createForm.value.password) {
    message.warning('请完整填写用户名与密码')
    return
  }
  if (createForm.value.password.length < 6) {
    message.warning('密码长度至少需要 6 位')
    return
  }

  submitting.value = true
  try {
    await api.createUser(createForm.value)
    message.success(`用户 ${createForm.value.username} 创建成功`)
    showCreateModal.value = false
    loadUsers()
  } catch (err: any) {
    message.error(err.response?.data?.error || '创建用户失败')
  } finally {
    submitting.value = false
  }
}

const openEditModal = (user: UserRecord) => {
  editingUser.value = user
  editForm.value = {
    role: user.role,
    password: '',
  }
  showEditModal.value = true
}

const handleUpdateUser = async () => {
  if (!editingUser.value) return

  submitting.value = true
  try {
    await api.updateUser(editingUser.value.id, {
      role: editForm.value.role,
      password: editForm.value.password || undefined,
    })
    message.success(`用户 ${editingUser.value.username} 更新成功`)
    showEditModal.value = false
    loadUsers()
  } catch (err: any) {
    message.error(err.response?.data?.error || '更新用户失败')
  } finally {
    submitting.value = false
  }
}

const handleDeleteUser = async (user: UserRecord) => {
  try {
    await api.deleteUser(user.id)
    message.success(`用户 ${user.username} 已删除`)
    loadUsers()
  } catch (err: any) {
    message.error(err.response?.data?.error || '删除失败')
  }
}

const columns: DataTableColumns<UserRecord> = [
  {
    title: '用户名',
    key: 'username',
    width: 150,
    render(row) {
      const isSelf = store.currentUser?.id === row.id
      return h('div', { class: 'user-name-cell' }, [
        h('strong', row.username),
        isSelf ? h(NTag, { size: 'tiny', type: 'info', class: 'ml-2', round: true }, { default: () => '当前账号' }) : null,
      ])
    },
  },
  {
    title: '用户角色',
    key: 'role',
    width: 170,
    render(row) {
      if (row.role === 'admin') {
        return h(
          NTag,
          { type: 'success', size: 'small', round: true },
          {
            default: () => [
              h(NIcon, { class: 'mr-1' }, { default: () => h(ShieldCheckmarkOutline) }),
              '管理员 (admin)',
            ],
          }
        )
      }
      return h(
        NTag,
        { type: 'default', size: 'small', round: true },
        {
          default: () => [
            h(NIcon, { class: 'mr-1' }, { default: () => h(EyeOutline) }),
            '只读用户 (readonly)',
          ],
        }
      )
    },
  },
  {
    title: '创建时间',
    key: 'created_at',
    width: 180,
    render(row) {
      return new Date(row.created_at).toLocaleString()
    },
  },
  {
    title: '操作',
    key: 'actions',
    width: 170,
    render(row) {
      const isSelf = store.currentUser?.id === row.id
      return h(NSpace, { size: 'small' }, {
        default: () => [
          h(
            NButton,
            {
              size: 'tiny',
              secondary: true,
              onClick: () => openEditModal(row),
            },
            { default: () => '编辑 / 改密' }
          ),
          h(
            NPopconfirm,
            {
              onPositiveClick: () => handleDeleteUser(row),
            },
            {
              trigger: () =>
                h(
                  NButton,
                  {
                    size: 'tiny',
                    type: 'error',
                    secondary: true,
                    disabled: isSelf,
                    title: isSelf ? '不能删除当前登录账号' : '',
                  },
                  { default: () => '删除' }
                ),
              default: () => `确定要删除用户 "${row.username}" 吗？此操作无法撤回。`,
            }
          ),
        ],
      })
    },
  },
]

onMounted(loadUsers)
</script>

<style scoped>
.page-container {
  padding: 24px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0;
  font-size: 20px;
  color: var(--text-title);
}

.sub-title {
  font-size: 13px;
  color: var(--text-muted);
}

.user-card {
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  box-shadow: var(--card-shadow);
}

.user-name-cell {
  display: flex;
  align-items: center;
}

.ml-2 {
  margin-left: 8px;
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: stretch !important;
    gap: 12px;
  }
  .page-header .n-button {
    width: 100% !important;
    justify-content: center !important;
  }
}
</style>
