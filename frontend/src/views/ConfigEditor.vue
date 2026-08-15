<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h2>接口与配置中心</h2>
        <span class="sub-title">OpenNHRP 接口配置、配置文件在线编辑与热重载、操作审计追溯</span>
      </div>
      <n-space align="center">
        <n-select
          v-model:value="store.activeNodeId"
          :options="store.nodeOptions"
          placeholder="选择配置节点"
          style="width: 250px;"
          @update:value="loadData"
        />
        <n-button
          type="warning"
          secondary
          :disabled="!store.isAdmin"
          :title="!store.isAdmin ? '只读用户无权操作' : ''"
          @click="handleReloadConfig"
        >
          热重载配置 (Reload)
        </n-button>
        <n-button
          type="primary"
          :loading="saving"
          :disabled="!store.isAdmin"
          :title="!store.isAdmin ? '只读用户无权操作' : ''"
          @click="handleSaveConfig"
        >
          保存配置文件 (Save)
        </n-button>
      </n-space>
    </div>

    <!-- Interfaces Table -->
    <n-card title="OpenNHRP 接口 (Interfaces)" class="mb-4">
      <n-scrollbar x-scrollable>
        <n-table :bordered="false" :single-line="true" style="min-width: 640px;">
          <thead>
            <tr>
              <th style="width: 140px;">接口名称 (Name)</th>
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
      </n-scrollbar>
    </n-card>

    <!-- Config Editor Card -->
    <n-card title="OpenNHRP 配置文件编辑 (opennhrp.conf)" class="mb-4">
      <div class="editor-wrapper">
        <n-input
          v-model:value="configContent"
          type="textarea"
          rows="14"
          placeholder="正在读取当前节点配置文件..."
          style="font-family: 'Fira Code', monospace; font-size: 13px;"
        />
      </div>
      <div class="editor-footer mt-2">
        <n-input
          v-model:value="saveComment"
          placeholder="可选：输入修改备注（将记录至审计日志）"
          style="width: 380px; max-width: 100%;"
        />
      </div>
    </n-card>

    <!-- Audit Logs -->
    <n-card title="配置操作与审计历史 (Audit Logs)">
      <n-scrollbar x-scrollable>
        <n-table :bordered="false" :single-line="true" size="small" style="min-width: 820px;">
          <thead>
            <tr>
              <th style="width: 170px;">操作时间</th>
              <th style="width: 160px;">节点 ID</th>
              <th style="width: 130px;">操作类型</th>
              <th style="width: 100px;">执行人</th>
              <th style="min-width: 180px;" class="allow-wrap">备注 / 详情</th>
              <th style="width: 90px;">结果</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="auditLogs.length === 0">
              <td colspan="6" class="text-center text-muted">暂无历史操作记录</td>
            </tr>
            <tr v-for="log in auditLogs" :key="log.id">
              <td>{{ new Date(log.created_at).toLocaleString() }}</td>
              <td><code>{{ log.node_id }}</code></td>
              <td><n-tag size="tiny" type="info">{{ log.action }}</n-tag></td>
              <td>{{ log.operator }}</td>
              <td class="allow-wrap">{{ log.detail || '-' }}</td>
              <td>
                <n-tag size="tiny" :type="log.success ? 'success' : 'error'">
                  {{ log.success ? 'SUCCESS' : 'FAILED' }}
                </n-tag>
              </td>
            </tr>
          </tbody>
        </n-table>
      </n-scrollbar>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import {
  NCard,
  NButton,
  NTable,
  NTag,
  NInput,
  NSelect,
  NScrollbar,
  NSpace,
  useMessage,
} from 'naive-ui'
import { api } from '../api/client'
import { useAppStore } from '../store'
import type { InterfaceInfo, AuditLog } from '../types'

const store = useAppStore()
const message = useMessage()
const interfaces = ref<InterfaceInfo[]>([])
const configContent = ref('')
const saveComment = ref('')
const saving = ref(false)
const auditLogs = ref<AuditLog[]>([])

const loadData = async () => {
  try {
    const targetNode = store.activeNodeId
    const [ifaces, conf, logs] = await Promise.all([
      api.listInterfaces(targetNode),
      api.getConfigFile(targetNode),
      api.getAuditLogs(20),
    ])
    interfaces.value = ifaces
    configContent.value = conf.content
    auditLogs.value = logs.items
  } catch (e) {
    console.error('Failed to load config center data', e)
  }
}

watch(
  () => store.activeNodeId,
  () => {
    loadData()
  }
)

const handleSaveConfig = async () => {
  saving.value = true
  try {
    const targetNode = store.activeNodeId
    await api.saveConfigFile(targetNode, {
      content: configContent.value,
      comment: saveComment.value || 'Web 控制台修改配置',
    })
    message.success('配置文件已成功写入节点并备份')
    saveComment.value = ''
    loadData()
  } catch (e: any) {
    message.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

const handleReloadConfig = async () => {
  try {
    const targetNode = store.activeNodeId
    await api.reloadConfig(targetNode)
    message.success('已通知 OpenNHRP 热重载配置')
    loadData()
  } catch (e: any) {
    message.error('热重载失败')
  }
}

onMounted(loadData)
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

.mb-4 {
  margin-bottom: 16px;
}

.mt-2 {
  margin-top: 8px;
}

.editor-wrapper {
  border-radius: 6px;
}

.editor-footer {
  display: flex;
  justify-content: flex-end;
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
  .editor-footer {
    justify-content: stretch;
  }
  .editor-footer .n-input {
    width: 100% !important;
  }
}
</style>
