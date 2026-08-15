<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h2>Spoke 客户端配置向导与打包中心</h2>
        <span class="sub-title">一键生成 Spoke 端 `opennhrp.conf`、网络初始化脚本与安全认证 Keyring 部署包</span>
      </div>
      <n-button type="primary" :loading="downloading" @click="handleDownloadZip">
        <template #icon><n-icon><CloudDownloadOutline /></n-icon></template>
        打包下载部署包 (.ZIP)
      </n-button>
    </div>

    <n-grid cols="1 m:12" responsive="screen" :x-gap="20" :y-gap="16">
      <!-- Left Config Form -->
      <n-grid-item span="1 m:5">
        <n-card title="Spoke 参数设置" class="form-card">
          <n-form label-placement="left" label-width="140">
            <n-form-item label="mGRE 接口名">
              <n-input v-model:value="form.interface_name" placeholder="gre-ha" @input="updatePreview" />
            </n-form-item>
            <n-form-item label="Spoke 隧道 IP">
              <n-input v-model:value="form.local_ip" placeholder="10.20.0.101/24" @input="updatePreview" />
            </n-form-item>
            <n-form-item label="Hub 共享网关 IP">
              <n-input v-model:value="form.hub_protocol_ip" placeholder="10.20.0.1" @input="updatePreview" />
            </n-form-item>
            <n-form-item label="Hub 1 外网 IP">
              <n-input v-model:value="hub1IP" placeholder="从当前 HA 成员自动读取" @input="updatePreview" />
            </n-form-item>
            <n-form-item label="Hub 2 外网 IP">
              <n-input v-model:value="hub2IP" placeholder="可选备用 Hub" @input="updatePreview" />
            </n-form-item>
            <n-form-item label="GRE Tunnel Key">
              <n-input v-model:value="form.gre_key" placeholder="可选，如 1021" @input="updatePreview" />
            </n-form-item>
          </n-form>
          <div class="tip-box">
            <strong>部署提示：</strong> 生成的配置将自动配置 <code>holding-time 300</code>、<code>shortcut</code> 及 <code>redirect</code>，Spoke 启动后将自动注册至当前活跃 Leader，并在主备切换时无缝重定向。
          </div>
        </n-card>
      </n-grid-item>

      <!-- Right Code Preview -->
      <n-grid-item span="1 m:7">
        <n-card class="preview-card">
          <n-tabs type="line">
            <n-tab-pane name="conf" tab="opennhrp.conf">
              <div class="code-header">
                <span class="text-muted">/etc/opennhrp/opennhrp.conf</span>
                <n-button size="tiny" secondary @click="copyText(previewConf)">复制配置</n-button>
              </div>
              <n-scrollbar x-scrollable style="max-height: 480px;">
                <pre class="code-block">{{ previewConf }}</pre>
              </n-scrollbar>
            </n-tab-pane>

            <n-tab-pane name="script" tab="setup-gre.sh (启动脚本)">
              <div class="code-header">
                <span class="text-muted">/usr/local/bin/setup-gre.sh</span>
                <n-button size="tiny" secondary @click="copyText(previewScript)">复制脚本</n-button>
              </div>
              <n-scrollbar x-scrollable style="max-height: 480px;">
                <pre class="code-block">{{ previewScript }}</pre>
              </n-scrollbar>
            </n-tab-pane>

            <n-tab-pane name="deploy" tab="三步极简部署说明">
              <div class="deploy-steps">
                <div class="step-item">
                  <div class="step-num">1</div>
                  <div class="step-content">
                    <h4>下载并解压部署包</h4>
                    <p>点击右上角“打包下载部署包”，解压至 Spoke 机器：<code>unzip opennhrp-spoke-package.zip -d /etc/opennhrp</code></p>
                  </div>
                </div>

                <div class="step-item">
                  <div class="step-num">2</div>
                  <div class="step-content">
                    <h4>执行隧道创建脚本</h4>
                    <p>赋予可执行权限并运行：<code>chmod +x setup-gre.sh && sudo ./setup-gre.sh</code></p>
                  </div>
                </div>

                <div class="step-item">
                  <div class="step-num">3</div>
                  <div class="step-content">
                    <h4>验证连通性</h4>
                    <p>在终端运行 <code>opennhrpctl show</code> 或 Ping 网关 <code>ping {{ form.hub_protocol_ip }}</code>，即可在 Web 控制台的“Spoke 列表”中看到该节点。</p>
                  </div>
                </div>
              </div>
            </n-tab-pane>
          </n-tabs>
        </n-card>
      </n-grid-item>
    </n-grid>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  NGrid,
  NGridItem,
  NCard,
  NForm,
  NFormItem,
  NInput,
  NButton,
  NTabs,
  NTabPane,
  NScrollbar,
  NIcon,
  useMessage,
} from 'naive-ui'
import { CloudDownloadOutline } from '@vicons/ionicons5'
import { api } from '../api/client'
import { useAppStore } from '../store'

const store = useAppStore()
const message = useMessage()
const downloading = ref(false)

const form = ref({
  interface_name: 'gre-ha',
  local_ip: '10.20.0.101/24',
  hub_protocol_ip: '10.20.0.1',
  gre_key: '',
})

const hub1IP = ref('')
const hub2IP = ref('')

const previewConf = ref('')
const previewScript = ref('')

const updatePreview = async () => {
  const endpoints: string[] = []
  if (hub1IP.value) endpoints.push(hub1IP.value)
  if (hub2IP.value) endpoints.push(hub2IP.value)

  try {
    const res = await api.generateSpokeConfig({
      ...form.value,
      hub_endpoints: endpoints,
    })
    previewConf.value = res.opennhrp_conf
    previewScript.value = res.setup_script
  } catch (e) {
    console.error('Failed to generate preview', e)
  }
}

const copyText = (text: string) => {
  navigator.clipboard.writeText(text)
  message.success('已复制到剪贴板')
}

const handleDownloadZip = async () => {
  downloading.value = true
  try {
    const endpoints: string[] = []
    if (hub1IP.value) endpoints.push(hub1IP.value)
    if (hub2IP.value) endpoints.push(hub2IP.value)

    const response = await fetch('/api/spokes/provision/download', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        ...form.value,
        hub_endpoints: endpoints,
      }),
    })

    const blob = await response.blob()
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `opennhrp-spoke-${form.value.interface_name}.zip`
    document.body.appendChild(a)
    a.click()
    a.remove()
    message.success('部署包下载成功')
  } catch (e) {
    message.error('下载失败')
  } finally {
    downloading.value = false
  }
}

onMounted(async () => {
  // Pre-populate with current members if available
  try {
    const members = await api.getMembers(store.activeNodeId)
    if (members.length >= 1 && members[0].advertised_addresses?.[0]) {
      hub1IP.value = members[0].advertised_addresses[0]
    }
    if (members.length >= 2 && members[1].advertised_addresses?.[0]) {
      hub2IP.value = members[1].advertised_addresses[0]
    }
  } catch (e) {}
  updatePreview()
})
</script>

<style scoped>
.page-container {
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-header h2 {
  margin: 0 0 4px 0;
  font-size: 22px;
  font-weight: 700;
  color: var(--text-title);
}

.sub-title {
  font-size: 13px;
  color: var(--text-muted);
}

.form-card, .preview-card {
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  box-shadow: var(--card-shadow);
  transition: background-color 0.2s ease, border-color 0.2s ease;
}

.tip-box {
  background: rgba(24, 160, 88, 0.08);
  border: 1px solid rgba(24, 160, 88, 0.25);
  padding: 12px;
  border-radius: 6px;
  font-size: 12px;
  line-height: 1.6;
  color: var(--text-body);
}

.code-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.code-block {
  background: #0f172a;
  border: 1px solid #1e293b;
  padding: 16px;
  border-radius: 8px;
  color: #a7f3d0;
  font-family: 'Fira Code', monospace;
  font-size: 13px;
  line-height: 1.6;
  overflow-x: auto;
}

.deploy-steps {
  padding: 10px 0;
}

.step-item {
  display: flex;
  gap: 16px;
  margin-bottom: 20px;
}

.step-num {
  width: 32px;
  height: 32px;
  background: #3b82f6;
  color: white;
  font-weight: bold;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.step-content h4 {
  margin: 0 0 6px 0;
  font-size: 14px;
  color: var(--text-title);
}

.step-content p {
  margin: 0;
  font-size: 13px;
  color: var(--text-muted);
}

.text-muted { color: var(--text-muted); }

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
