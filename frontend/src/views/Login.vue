<template>
  <div class="login-wrapper">
    <!-- Top Right Theme Switcher -->
    <div class="theme-toggle">
      <n-button size="small" secondary circle @click="store.toggleTheme">
        <n-icon><component :is="store.isDark ? MoonOutline : SunnyOutline" /></n-icon>
      </n-button>
    </div>

    <div class="login-card-container">
      <div class="login-card">
        <!-- Logo & Branding -->
        <div class="brand-header">
          <div class="logo-icon">NHRP</div>
          <div class="brand-titles">
            <h1 class="main-title">OpenNHRP Manager</h1>
            <p class="sub-title">高可用 DMVPN 拓扑控制与数据面中枢</p>
          </div>
        </div>

        <!-- Error Alert -->
        <n-alert v-if="errorMessage" type="error" closable class="mb-4" @close="errorMessage = ''">
          {{ errorMessage }}
        </n-alert>

        <!-- Login Form -->
        <n-form ref="formRef" :model="form" :rules="rules" @submit.prevent="handleLogin">
          <n-form-item label="用户名" path="username">
            <n-input
              v-model:value="form.username"
              placeholder="请输入管理员或只读用户名"
              size="large"
              :autofocus="true"
              @keydown.enter="handleLogin"
            >
              <template #prefix>
                <n-icon class="input-icon"><PersonOutline /></n-icon>
              </template>
            </n-input>
          </n-form-item>

          <n-form-item label="登录密码" path="password">
            <n-input
              v-model:value="form.password"
              type="password"
              show-password-on="click"
              placeholder="请输入登录密码"
              size="large"
              @keydown.enter="handleLogin"
            >
              <template #prefix>
                <n-icon class="input-icon"><LockClosedOutline /></n-icon>
              </template>
            </n-input>
          </n-form-item>

          <n-button
            type="primary"
            block
            size="large"
            :loading="loading"
            class="login-btn"
            @click="handleLogin"
          >
            登 录 系 统
          </n-button>
        </n-form>

      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import {
  NForm,
  NFormItem,
  NInput,
  NButton,
  NAlert,
  NIcon,
  createDiscreteApi,
  darkTheme,
} from 'naive-ui'
import { LockClosedOutline, MoonOutline, PersonOutline, SunnyOutline } from '@vicons/ionicons5'
import { useAppStore } from '../store'

const router = useRouter()
const route = useRoute()
const store = useAppStore()

const { message } = createDiscreteApi(['message'], {
  configProviderProps: computed(() => ({
    theme: store.isDark ? darkTheme : null,
  })),
})

const form = ref({
  username: '',
  password: '',
})

const loading = ref(false)
const errorMessage = ref('')

const rules = {
  username: {
    required: true,
    message: '请输入用户名',
    trigger: 'blur',
  },
  password: {
    required: true,
    message: '请输入登录密码',
    trigger: 'blur',
  },
}

const handleLogin = async () => {
  if (!form.value.username || !form.value.password) {
    errorMessage.value = '请完整填写用户名与密码'
    return
  }

  loading.value = true
  errorMessage.value = ''

  try {
    const res = await store.login(form.value.username, form.value.password)
    message.success(`欢迎回来，${res.user.username} (${res.user.role === 'admin' ? '管理员' : '只读用户'})`)
    
    const redirectUrl = (route.query.redirect as string) || '/'
    router.push(redirectUrl)
  } catch (err: any) {
    const msg = err.response?.data?.error || '登录验证失败，请检查用户名与密码'
    errorMessage.value = msg
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-wrapper {
  position: relative;
  min-height: 100vh;
  width: 100vw;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: var(--bg-body, #09090b);
  background-image: 
    radial-gradient(at 0% 0%, rgba(24, 160, 88, 0.12) 0px, transparent 50%),
    radial-gradient(at 100% 100%, rgba(59, 130, 246, 0.1) 0px, transparent 50%);
  padding: 20px;
  box-sizing: border-box;
  overflow: hidden;
}

.theme-toggle {
  position: absolute;
  top: 24px;
  right: 24px;
  z-index: 10;
}

.login-card-container {
  width: 100%;
  max-width: 440px;
}

.login-card {
  background: var(--bg-card, #121214);
  border: 1px solid var(--border-color, rgba(255, 255, 255, 0.08));
  border-radius: 16px;
  padding: 36px 32px;
  box-shadow: var(--card-shadow, 0 10px 40px rgba(0, 0, 0, 0.4));
  backdrop-filter: blur(12px);
  transition: all 0.25s ease;
}

.brand-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 28px;
}

.logo-icon {
  background: #18a058;
  color: white;
  font-weight: 900;
  font-size: 15px;
  padding: 10px 14px;
  border-radius: 10px;
  letter-spacing: 1.5px;
  box-shadow: 0 4px 14px rgba(24, 160, 88, 0.35);
}

.brand-titles .main-title {
  margin: 0 0 4px 0;
  font-size: 20px;
  font-weight: 800;
  color: var(--text-title, #f4f4f5);
  letter-spacing: -0.3px;
}

.brand-titles .sub-title {
  margin: 0;
  font-size: 12px;
  color: var(--text-muted, #71717a);
}

.input-icon {
  font-size: 14px;
  margin-right: 4px;
  opacity: 0.8;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  vertical-align: middle;
}

.login-btn {
  margin-top: 10px;
  font-weight: 700;
  letter-spacing: 2px;
  height: 44px;
  border-radius: 8px;
}

.mb-4 {
  margin-bottom: 16px;
}

@media (max-width: 768px) {
  .login-card {
    padding: 24px 18px;
  }
  .theme-toggle {
    top: 14px;
    right: 14px;
  }
}
</style>
