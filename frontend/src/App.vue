<template>
  <n-config-provider :theme="store.isDark ? darkTheme : null">
    <n-loading-bar-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <n-message-provider>
            <!-- When on login page, render full screen login view without sidebar and header -->
            <router-view v-if="isLoginPage" />

            <!-- When authenticated inside management console -->
            <n-layout v-else :has-sider="!isMobile" class="app-layout" :native-scrollbar="false">
              <!-- Desktop Sidebar -->
              <n-layout-sider
                v-if="!isMobile"
                bordered
                collapse-mode="width"
                :collapsed-width="64"
                :width="220"
                :native-scrollbar="false"
                class="app-sider"
              >
                <div class="brand-logo">
                  <div class="logo-icon">NHRP</div>
                  <div class="logo-text">
                    <div class="title">OpenNHRP</div>
                    <div class="badge">MANAGER</div>
                  </div>
                </div>

                <n-scrollbar style="max-height: calc(100vh - 65px);">
                  <n-menu
                    :value="currentRoute"
                    :options="menuOptions"
                    @update:value="handleMenuSelect"
                  />
                </n-scrollbar>
              </n-layout-sider>

              <!-- Mobile Drawer Sidebar -->
              <n-drawer v-model:show="mobileMenuOpen" placement="left" :width="260">
                <n-drawer-content :native-scrollbar="false" body-content-style="padding: 0;">
                  <div class="brand-logo">
                    <div class="logo-icon">NHRP</div>
                    <div class="logo-text">
                      <div class="title">OpenNHRP</div>
                      <div class="badge">MANAGER</div>
                    </div>
                  </div>
                  <n-menu
                    :value="currentRoute"
                    :options="menuOptions"
                    @update:value="(key) => { handleMenuSelect(key); mobileMenuOpen = false; }"
                  />
                </n-drawer-content>
              </n-drawer>

              <!-- Main Content Area -->
              <n-layout class="main-layout" :native-scrollbar="false">
                <n-layout-header bordered class="app-header">
                  <div class="header-left">
                    <n-button
                      v-if="isMobile"
                      quaternary
                      circle
                      size="small"
                      class="mobile-menu-btn mr-2"
                      @click="mobileMenuOpen = true"
                    >
                      <n-icon><MenuOutline /></n-icon>
                    </n-button>
                    <div class="header-breadcrumb">
                      <span class="header-console-title">{{ isMobile ? 'OpenNHRP' : 'OpenNHRP 分布式管控中心' }}</span>
                    </div>
                  </div>
                  <div class="header-actions">
                    <n-tag
                      v-if="!isMobile"
                      type="success"
                      size="small"
                      round
                      style="cursor: pointer;"
                      title="点击跳转至 Witness 见证仲裁与 SLA 质量中心"
                      @click="router.push('/witness')"
                    >
                      <n-icon><ShieldCheckmarkOutline /></n-icon>
                      见证仲裁 Active
                    </n-tag>

                    <!-- User Profile Dropdown -->
                    <n-dropdown
                      trigger="click"
                      :options="userDropdownOptions"
                      @select="handleUserDropdownSelect"
                    >
                      <n-button size="small" secondary round class="user-btn">
                        <n-icon class="mr-1"><component :is="store.isAdmin ? ShieldCheckmarkOutline : EyeOutline" /></n-icon>
                        <span class="user-name">{{ store.currentUser?.username || '用户' }}</span>
                        <n-tag v-if="!isMobile" size="tiny" :type="store.isAdmin ? 'success' : 'default'" round class="ml-1">
                          {{ store.isAdmin ? '管理员' : '只读' }}
                        </n-tag>
                      </n-button>
                    </n-dropdown>

                    <n-button size="small" secondary circle @click="store.toggleTheme">
                      <n-icon><component :is="store.isDark ? MoonOutline : SunnyOutline" /></n-icon>
                    </n-button>
                  </div>
                </n-layout-header>

                <n-layout-content class="app-content" :native-scrollbar="false">
                  <n-scrollbar style="max-height: calc(100vh - 56px);">
                    <router-view />
                  </n-scrollbar>
                </n-layout-content>
              </n-layout>
            </n-layout>

            <!-- Change Password Modal -->
            <n-modal
              v-model:show="showPasswordModal"
              preset="card"
              title="修改密码"
              style="width: 440px; max-width: calc(100vw - 32px);"
              :bordered="false"
            >
              <n-form ref="pwFormRef" :model="pwForm">
                <n-form-item label="当前原密码" path="old_password">
                  <n-input
                    v-model:value="pwForm.old_password"
                    type="password"
                    show-password-on="click"
                    placeholder="请输入当前使用的旧密码"
                  />
                </n-form-item>
                <n-form-item label="新密码" path="new_password">
                  <n-input
                    v-model:value="pwForm.new_password"
                    type="password"
                    show-password-on="click"
                    placeholder="请输入新密码（至少 6 位）"
                  />
                </n-form-item>
                <n-form-item label="确认新密码" path="confirm_password">
                  <n-input
                    v-model:value="pwForm.confirm_password"
                    type="password"
                    show-password-on="click"
                    placeholder="请再次输入新密码"
                  />
                </n-form-item>
              </n-form>
              <template #footer>
                <n-space justify="end">
                  <n-button @click="showPasswordModal = false">取消</n-button>
                  <n-button type="primary" :loading="pwSubmitting" @click="handleChangePassword">
                    确认修改
                  </n-button>
                </n-space>
              </template>
            </n-modal>
          </n-message-provider>
        </n-notification-provider>
      </n-dialog-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { ref, computed, h, onMounted, onUnmounted, type Component } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  NConfigProvider,
  NLoadingBarProvider,
  NDialogProvider,
  NNotificationProvider,
  NMessageProvider,
  NLayout,
  NLayoutSider,
  NLayoutHeader,
  NLayoutContent,
  NDrawer,
  NDrawerContent,
  NScrollbar,
  NMenu,
  NTag,
  NButton,
  NDropdown,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NSpace,
  NIcon,
  createDiscreteApi,
  darkTheme,
} from 'naive-ui'
import {
  ConstructOutline,
  EyeOutline,
  GitNetworkOutline,
  KeyOutline,
  LogOutOutline,
  MenuOutline,
  MoonOutline,
  PeopleOutline,
  PulseOutline,
  SettingsOutline,
  ShieldCheckmarkOutline,
  SpeedometerOutline,
  SunnyOutline,
} from '@vicons/ionicons5'
import { useAppStore } from './store'
import { api } from './api/client'

const router = useRouter()
const route = useRoute()
const store = useAppStore()

const { message } = createDiscreteApi(['message'], {
  configProviderProps: computed(() => ({
    theme: store.isDark ? darkTheme : null,
  })),
})

const isLoginPage = computed(() => route.path === '/login')
const currentRoute = computed(() => route.path)

const showPasswordModal = ref(false)
const pwSubmitting = ref(false)
const pwForm = ref({
  old_password: '',
  new_password: '',
  confirm_password: '',
})

const renderIcon = (icon: Component) => () => h(NIcon, null, { default: () => h(icon) })

const userDropdownOptions = computed(() => {
  return [
    { label: '修改密码', key: 'password', icon: renderIcon(KeyOutline) },
    { label: '退出登录', key: 'logout', icon: renderIcon(LogOutOutline) },
  ]
})

const handleUserDropdownSelect = (key: string) => {
  if (key === 'password') {
    pwForm.value = {
      old_password: '',
      new_password: '',
      confirm_password: '',
    }
    showPasswordModal.value = true
  } else if (key === 'logout') {
    store.logout()
  }
}

const handleChangePassword = async () => {
  if (!pwForm.value.old_password || !pwForm.value.new_password) {
    message.warning('请填写原密码与新密码')
    return
  }
  if (pwForm.value.new_password.length < 6) {
    message.warning('新密码长度至少需要 6 位')
    return
  }
  if (pwForm.value.new_password !== pwForm.value.confirm_password) {
    message.warning('两次输入的新密码不一致')
    return
  }

  pwSubmitting.value = true
  try {
    await api.changePassword({
      old_password: pwForm.value.old_password,
      new_password: pwForm.value.new_password,
    })
    message.success('密码修改成功')
    showPasswordModal.value = false
  } catch (err: any) {
    message.error(err.response?.data?.error || '修改密码失败')
  } finally {
    pwSubmitting.value = false
  }
}


const menuOptions = computed(() => {
  const list = [
    {
      label: '仪表盘概览',
      key: '/',
      icon: renderIcon(SpeedometerOutline),
    },
    {
      label: 'Hub HA 集群管理',
      key: '/ha',
      icon: renderIcon(GitNetworkOutline),
    },
    {
      label: 'Spoke 客户端管理',
      key: '/spokes',
      icon: renderIcon(PulseOutline),
    },
    {
      label: 'Spoke 设备管理',
      key: '/managed-spokes',
      icon: renderIcon(SettingsOutline),
    },
    {
      label: 'Spoke 配置生成向导',
      key: '/provisioning',
      icon: renderIcon(ConstructOutline),
    },
    {
      label: 'Witness 仲裁与 SLA',
      key: '/witness',
      icon: renderIcon(ShieldCheckmarkOutline),
    },
    {
      label: '接口与配置中心',
      key: '/config',
      icon: renderIcon(SettingsOutline),
    },
  ]

  if (store.isAdmin) {
    list.push({
      label: '用户与权限管理',
      key: '/users',
      icon: renderIcon(PeopleOutline),
    })
  }

  return list
})

const isMobile = ref(typeof window !== 'undefined' ? window.innerWidth <= 768 : false)
const mobileMenuOpen = ref(false)

const checkMobile = () => {
  if (typeof window !== 'undefined') {
    isMobile.value = window.innerWidth <= 768
    if (!isMobile.value) {
      mobileMenuOpen.value = false
    }
  }
}

const handleMenuSelect = (key: string) => {
  router.push(key)
}

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
  if (store.isLoggedIn) {
    store.checkAuth()
    store.fetchNodes()
  }
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<style>
:root {
  --bg-body: #f4f6f8;
  --bg-sider: #ffffff;
  --bg-header: #ffffff;
  --bg-content: #f4f6f8;
  --bg-card: #ffffff;
  --bg-card-secondary: #f1f5f9;
  --bg-code: #f1f5f9;
  --text-title: #0f172a;
  --text-body: #334155;
  --text-muted: #64748b;
  --border-color: #e2e8f0;
  --card-shadow: 0 1px 3px rgba(0, 0, 0, 0.05), 0 1px 2px rgba(0, 0, 0, 0.03);
  --code-color: #0f766e;
}

html.dark, :root.dark {
  --bg-body: #09090b;
  --bg-sider: #121214;
  --bg-header: #121214;
  --bg-content: #09090b;
  --bg-card: #121214;
  --bg-card-secondary: #18181b;
  --bg-code: rgba(255, 255, 255, 0.06);
  --text-title: #f4f4f5;
  --text-body: #e4e4e7;
  --text-muted: #71717a;
  --border-color: rgba(255, 255, 255, 0.08);
  --card-shadow: 0 4px 20px rgba(0, 0, 0, 0.35);
  --code-color: #a7f3d0;
}

body {
  margin: 0;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background-color: var(--bg-body);
  color: var(--text-body);
  transition: background-color 0.2s ease, color 0.2s ease;
  overflow: hidden;
}

code {
  font-family: 'Fira Code', monospace;
  background: var(--bg-code);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
  color: var(--code-color);
}

/* Fallback & Custom Scrollbar Theme Reset */
* {
  scrollbar-width: thin;
  scrollbar-color: rgba(140, 140, 140, 0.25) transparent;
}

::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}

::-webkit-scrollbar-track {
  background: transparent;
}

::-webkit-scrollbar-thumb {
  background: rgba(140, 140, 140, 0.25);
  border-radius: 3px;
}

/* Global icon vertical centering and alignment */
.n-icon {
  display: inline-flex !important;
  align-items: center !important;
  justify-content: center !important;
  vertical-align: -0.15em !important;
  line-height: 1 !important;
}

.n-icon svg {
  display: block;
  width: 1em;
  height: 1em;
}

/* Ensure tag content aligns icon and text vertically centered */
.n-tag .n-tag__content {
  display: inline-flex !important;
  align-items: center !important;
  vertical-align: middle;
  gap: 4px;
}

.n-tag .n-tag__content .n-icon {
  vertical-align: 0 !important;
}

/* Ensure button content aligns icon and text vertically centered */
.n-button .n-button__content {
  display: inline-flex !important;
  align-items: center !important;
  vertical-align: middle;
}

.n-button .n-button__icon {
  display: inline-flex !important;
  align-items: center !important;
}

.n-button .n-button__icon .n-icon {
  vertical-align: 0 !important;
}

/* Menu items and dropdown options vertical centering */
.n-menu-item-content,
.n-dropdown-option-body {
  display: flex !important;
  align-items: center !important;
}

.n-menu-item-content__icon,
.n-dropdown-option-body__icon {
  display: inline-flex !important;
  align-items: center !important;
}

/* Common icon helper classes */
.icon-value,
.online-tag,
.target-item,
.info-tip {
  display: inline-flex !important;
  align-items: center !important;
  vertical-align: middle !important;
  gap: 4px;
}

.icon-value .n-icon,
.online-tag .n-icon,
.target-item .n-icon,
.info-tip .n-icon {
  vertical-align: 0 !important;
}
</style>

<style scoped>
.app-layout {
  height: 100vh;
  overflow: hidden;
}

.app-sider {
  background: var(--bg-sider) !important;
  border-right: 1px solid var(--border-color) !important;
  transition: background-color 0.2s ease;
}

.brand-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 18px 16px;
  border-bottom: 1px solid var(--border-color);
}

.logo-icon {
  background: #18a058;
  color: white;
  font-weight: 800;
  font-size: 12px;
  padding: 6px 8px;
  border-radius: 6px;
  letter-spacing: 1px;
}

.logo-text .title {
  font-weight: 700;
  font-size: 15px;
  color: var(--text-title);
}

.logo-text .badge {
  font-size: 9px;
  color: var(--text-muted);
  letter-spacing: 1.5px;
  font-weight: 600;
}

.main-layout {
  background: var(--bg-body);
  overflow: hidden;
}

.app-header {
  height: 56px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 24px;
  background: var(--bg-header) !important;
  border-bottom: 1px solid var(--border-color) !important;
  transition: background-color 0.2s ease;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.mobile-menu-btn {
  font-size: 18px;
  padding: 4px;
  line-height: 1;
}

.header-breadcrumb {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-console-title {
  font-weight: 700;
  font-size: 15px;
  color: var(--text-title);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.user-btn {
  display: flex;
  align-items: center;
  font-weight: 600;
}

.user-name {
  max-width: 90px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mr-1 {
  margin-right: 4px;
}

.mr-2 {
  margin-right: 8px;
}

.ml-1 {
  margin-left: 6px;
}

.app-content {
  background: var(--bg-content);
  overflow: hidden;
  height: calc(100vh - 56px);
  transition: background-color 0.2s ease;
}

/* All tables unified: clean horizontal lines, NO vertical divider lines, no wrapping by default, GLOBAL STICKY HEADER */
.n-table {
  border: none !important;
}

.n-table thead,
.n-table thead tr {
  position: sticky !important;
  top: 0 !important;
  z-index: 10 !important;
}

.n-table thead th,
.n-table th {
  position: sticky !important;
  top: 0 !important;
  background: var(--bg-card) !important;
  z-index: 10 !important;
  border-right: none !important;
  border-left: none !important;
  border-bottom: 1px solid var(--border-color) !important;
  white-space: nowrap;
  box-shadow: 0 1px 0 var(--border-color) !important;
}

.n-table td {
  border-right: none !important;
  border-left: none !important;
  border-bottom: 1px solid var(--border-color) !important;
  white-space: nowrap;
}

.n-table td.allow-wrap,
.n-table th.allow-wrap {
  white-space: normal;
}

.n-data-table {
  border: none !important;
}

.n-data-table .n-data-table-th {
  position: sticky !important;
  top: 0 !important;
  background: var(--bg-card) !important;
  z-index: 10 !important;
  border-right: none !important;
  border-left: none !important;
  border-bottom: 1px solid var(--border-color) !important;
  white-space: nowrap;
  box-shadow: 0 1px 0 var(--border-color) !important;
}

.n-data-table .n-data-table-td {
  border-right: none !important;
  border-left: none !important;
  border-bottom: 1px solid var(--border-color) !important;
  white-space: nowrap;
}

/* Mobile Responsiveness Rules */
@media (max-width: 768px) {
  .app-header {
    padding: 0 12px !important;
  }
  .page-container {
    padding: 10px 12px !important;
  }
  .header-console-title {
    font-size: 14px !important;
    font-weight: 700 !important;
  }
  .user-name {
    max-width: 60px;
  }
}
</style>
