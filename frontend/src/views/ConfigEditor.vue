<template>
  <div class="page-container config-page">
    <div class="page-header">
      <div>
        <h2>OpenNHRP 配置与操作</h2>
        <span class="sub-title">统一管理 Hub 与 Spoke 的接口、配置文件、Map 和进程缓存</span>
      </div>
      <n-space align="center">
        <n-select
          v-model:value="selectedNodeId"
          :options="processNodeOptions"
          placeholder="选择 OpenNHRP 节点"
          style="width: 250px;"
        />
      </n-space>
    </div>

    <!-- Interfaces Table -->
    <n-card title="OpenNHRP 接口 (Interfaces)" class="mb-4 interfaces-card">
      <div class="interfaces-table-scroll">
        <n-table :bordered="false" :single-line="true" style="min-width: 640px;">
          <thead>
            <tr>
              <th>接口名称 (Name)</th>
              <th style="width: 100px;">类型</th>
              <th style="width: 150px;">Protocol IP</th>
              <th style="width: 160px;">NBMA 外网物理地址</th>
              <th style="width: 90px;">MTU</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="interfaces.length === 0">
              <td colspan="5" class="text-center text-muted">正在加载接口数据...</td>
            </tr>
            <tr v-for="iface in interfaces" :key="iface.name">
              <td><strong>{{ iface.name }}</strong></td>
              <td><n-tag size="small" type="info">{{ iface.type || '-' }}</n-tag></td>
              <td><code>{{ iface.protocol_address || '-' }}</code></td>
              <td><code>{{ iface.nbma_address || '-' }}</code></td>
              <td>{{ iface.mtu || '-' }}</td>
            </tr>
          </tbody>
        </n-table>
      </div>
    </n-card>

    <!-- Config Editor Card -->
    <n-card title="OpenNHRP 配置文件编辑 (opennhrp.conf)" class="editor-card">
      <template #header-extra>
        <n-space class="editor-actions" :wrap="true" justify="end">
          <n-button type="primary" secondary :disabled="!canOperate" @click="showAddMapModal = true">添加静态映射</n-button>
          <n-popconfirm :disabled="!canOperate || !operationInterface" @positive-click="handleSaveMap">
            <template #trigger>
              <n-button type="warning" secondary :disabled="!canOperate || !operationInterface">保存 Map</n-button>
            </template>
            确认在 {{ selectedNode?.name || selectedNodeId }} 的 {{ operationInterface }} 接口持久化当前 Map？
          </n-popconfirm>
          <n-popconfirm :disabled="!canOperate" @positive-click="handlePurgeRedirect">
            <template #trigger>
              <n-button type="error" secondary :disabled="!canOperate">清除重定向缓存</n-button>
            </template>
            确认清理 {{ selectedNode?.name || selectedNodeId }} 进程的全部重定向与限流缓存？
          </n-popconfirm>
          <n-button
            type="warning"
            secondary
            :disabled="!canOperate"
            :title="!store.isAdmin ? '只读用户无权操作' : ''"
            @click="handleReloadConfig"
          >
            热重载配置 (Reload)
          </n-button>
          <n-button
            type="primary"
            :loading="saving"
            :disabled="!canOperate"
            :title="!store.isAdmin ? '只读用户无权操作' : ''"
            @click="handleSaveConfig"
          >
            保存配置文件 (Save)
          </n-button>
        </n-space>
      </template>
      <div class="editor-wrapper">
        <n-input
          class="config-input"
          v-model:value="configContent"
          type="textarea"
          :autosize="false"
          placeholder="正在读取当前节点配置文件..."
          style="font-family: 'Fira Code', monospace; font-size: 13px;"
        />
      </div>
    </n-card>

    <n-modal v-model:show="showAddMapModal" preset="card" title="添加静态 NHRP 映射" style="width: 480px; max-width: calc(100vw - 32px);">
      <n-form label-placement="left" label-width="120">
        <n-form-item label="目标节点"><n-input :value="`${selectedNode?.name || selectedNodeId} (${selectedNodeId})`" disabled /></n-form-item>
        <n-form-item label="隧道接口" required><n-select v-model:value="mapForm.interface" :options="interfaceOptions" /></n-form-item>
        <n-form-item label="Protocol IP" required><n-input v-model:value="mapForm.protocol_address" /></n-form-item>
        <n-form-item label="NBMA 地址" required><n-input v-model:value="mapForm.nbma_address" /></n-form-item>
        <n-form-item label="自动注册"><n-switch v-model:value="mapForm.register" /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAddMapModal = false">取消</n-button>
          <n-button type="primary" :loading="operating" @click="handleAddMap">添加</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, watch } from 'vue'
import {
  NCard,
  NButton,
  NForm,
  NFormItem,
  NTable,
  NTag,
  NInput,
  NModal,
  NPopconfirm,
  NSelect,
  NSpace,
  NSwitch,
  useMessage,
} from 'naive-ui'
import { api } from '../api/client'
import { useAppStore } from '../store'
import { isHubNode } from '../utils/topologyStatus'
import type { InterfaceInfo } from '../types'

const store = useAppStore()
const message = useMessage()
const selectedNodeId = ref('')
const interfaces = ref<InterfaceInfo[]>([])
const configContent = ref('')
const loadedConfigContent = ref('')
const saving = ref(false)
const operating = ref(false)
const showAddMapModal = ref(false)
const operationInterface = ref('')
const mapForm = ref({ interface: '', protocol_address: '', nbma_address: '', register: true })
const processNodes = computed(() => store.nodes.filter((node) => node.type === 'spoke' || isHubNode(node)))
const selectedNode = computed(() => processNodes.value.find((node) => node.id === selectedNodeId.value))
const canOperate = computed(() => store.isAdmin && selectedNode.value?.status !== 'offline' && !!selectedNodeId.value)
const processNodeOptions = computed(() => processNodes.value.map((node) => ({
  label: `${node.name || node.id} (${node.type === 'spoke' ? 'Spoke' : 'Hub'} · ${node.status === 'online' ? '在线' : node.status === 'degraded' ? '降级' : '离线'})`,
  value: node.id,
  disabled: node.status === 'offline',
})))
const interfaceOptions = computed(() => interfaces.value.map((item) => ({ label: item.name, value: item.name })))

const ensureSelectedNode = () => {
  if (processNodes.value.some((node) => node.id === selectedNodeId.value && node.status !== 'offline')) return
  selectedNodeId.value = processNodes.value.find((node) => node.id === store.activeNodeId && node.status !== 'offline')?.id
    || processNodes.value.find((node) => node.status !== 'offline')?.id
    || ''
}

const loadData = async (preserveDraft = false) => {
  try {
    const targetNode = selectedNodeId.value
    if (!targetNode) return
    const [ifaces, conf] = await Promise.all([
      api.listInterfaces(targetNode),
      api.getConfigFile(targetNode),
    ])
    interfaces.value = ifaces
    if (!ifaces.some((item) => item.name === operationInterface.value)) operationInterface.value = ifaces[0]?.name || ''
    if (!ifaces.some((item) => item.name === mapForm.value.interface)) mapForm.value.interface = ifaces[0]?.name || ''
    if (!preserveDraft || configContent.value === loadedConfigContent.value) configContent.value = conf.content
    loadedConfigContent.value = conf.content
  } catch (e) {
    console.error('Failed to load config center data', e)
  }
}

watch(
  selectedNodeId,
  () => {
    loadData()
  }
)

watch(processNodes, ensureSelectedNode)

const handleSaveConfig = async () => {
  saving.value = true
  try {
    const targetNode = selectedNodeId.value
    await api.saveConfigFile(targetNode, {
      content: configContent.value,
      comment: 'Web 控制台修改配置',
    })
    message.success('配置文件已成功写入节点并备份')
    loadData()
  } catch (e: any) {
    message.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

const handleReloadConfig = async () => {
  try {
    const targetNode = selectedNodeId.value
    await api.reloadConfig(targetNode)
    message.success('已通知 OpenNHRP 热重载配置')
    loadData()
  } catch (e: any) {
    message.error('热重载失败')
  }
}

const handleAddMap = async () => {
  if (!mapForm.value.interface || !mapForm.value.protocol_address || !mapForm.value.nbma_address) {
    message.error('请填写完整的接口和地址')
    return
  }
  operating.value = true
  try {
    await api.addStaticMap(selectedNodeId.value, mapForm.value)
    message.success('静态 Map 添加成功')
    showAddMapModal.value = false
    mapForm.value.protocol_address = ''
    mapForm.value.nbma_address = ''
    await loadData(true)
  } catch (e: any) {
    message.error(e.response?.data?.error || '添加失败')
  } finally {
    operating.value = false
  }
}

const handleSaveMap = async () => {
  try {
    await api.saveMap(selectedNodeId.value, operationInterface.value)
    message.success('当前 Map 已持久化')
    await loadData(true)
  } catch (e: any) {
    message.error(e.response?.data?.error || '保存 Map 失败')
  }
}

const handlePurgeRedirect = async () => {
  try {
    await api.purgeRedirect(selectedNodeId.value)
    message.success('重定向与限流缓存已清除')
    await loadData(true)
  } catch (e: any) {
    message.error(e.response?.data?.error || '清除缓存失败')
  }
}

onMounted(() => {
  ensureSelectedNode()
  loadData()
})
</script>

<style scoped>
.page-container {
  box-sizing: border-box;
  display: flex;
  flex-direction: column;
  height: calc(100vh - 56px);
  min-height: 0;
  overflow: hidden;
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

.mb-4 {
  margin-bottom: 16px;
}

.interfaces-card {
  flex: 0 0 auto;
}

.interfaces-table-scroll {
  overflow-x: auto;
}

.editor-card {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  flex-direction: column;
}

.editor-card :deep(.n-card-content) {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  flex-direction: column;
}

.editor-wrapper {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  border-radius: 6px;
}

.editor-wrapper :deep(.config-input),
.editor-wrapper :deep(.n-input),
.editor-wrapper :deep(.n-input-wrapper),
.editor-wrapper :deep(.n-input__textarea),
.editor-wrapper :deep(.n-input__textarea .n-scrollbar-container),
.editor-wrapper :deep(.n-input__textarea .n-scrollbar-content),
.editor-wrapper :deep(textarea) {
  height: 100% !important;
  min-height: 0;
}

.editor-wrapper :deep(textarea) {
  resize: none;
}

.text-center {
  text-align: center;
}

.text-muted {
  color: var(--text-muted);
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: stretch !important;
    gap: 12px;
  }
  .page-header .n-space {
    width: 100%;
    flex-direction: column !important;
    align-items: stretch !important;
    gap: 8px !important;
  }
  .page-header .n-space > * {
    width: 100% !important;
  }
  .page-header .n-button,
  .page-header .n-select {
    width: 100% !important;
    justify-content: center !important;
  }
  .editor-actions {
    justify-content: stretch;
  }
  .editor-actions > * {
    flex: 1 1 auto;
  }
}
</style>
