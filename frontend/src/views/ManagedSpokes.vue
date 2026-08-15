<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h2>Spoke 设备管理</h2>
        <span class="sub-title">独立 Agent 纳管、运行状态、Hub peer、配置与日志</span>
      </div>
      <n-space>
        <n-button secondary @click="loadSpokes">刷新</n-button>
        <n-button type="primary" :disabled="!store.isAdmin" @click="showCreate = true">登记 Spoke</n-button>
      </n-space>
    </div>

    <n-card class="mb-4">
      <n-scrollbar x-scrollable>
        <n-table :bordered="false" style="min-width: 850px">
          <thead><tr><th>名称 / ID</th><th>Protocol IP</th><th>Agent</th><th>OpenNHRP</th><th>Hub peers</th><th>RTT</th><th>最后心跳</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-if="spokes.length === 0"><td colspan="8" class="empty">暂无已登记的 Spoke</td></tr>
            <tr v-for="spoke in spokes" :key="spoke.id" :class="{ selected: selectedId === spoke.id }" @click="selectSpoke(spoke.id)">
              <td><strong>{{ spoke.name }}</strong><br><code>{{ spoke.id }}</code></td>
              <td><code>{{ spoke.protocol_address || '-' }}</code></td>
              <td><n-tag size="small" :type="spoke.status === 'online' ? 'success' : 'default'">{{ spoke.status === 'online' ? '在线' : '离线' }}</n-tag></td>
              <td><n-tag size="small" :type="spoke.core_available ? 'success' : 'error'">{{ spoke.core_available ? '可用' : '不可用' }}</n-tag></td>
              <td>{{ spoke.peer_count }}</td>
              <td>{{ spoke.ws_rtt_ms ? `${spoke.ws_rtt_ms.toFixed(1)} ms` : '-' }}</td>
              <td>{{ formatTime(spoke.last_seen) }}</td>
              <td @click.stop>
                <n-space>
                  <n-button size="tiny" secondary @click="selectSpoke(spoke.id)">管理</n-button>
                  <n-popconfirm :disabled="!store.isAdmin" @positive-click="rotateToken(spoke.id)">
                    <template #trigger><n-button size="tiny" warning secondary :disabled="!store.isAdmin">轮换令牌</n-button></template>
                    轮换后当前 Agent 会立即断开，确定继续？
                  </n-popconfirm>
                  <n-popconfirm :disabled="!store.isAdmin" @positive-click="deleteSpoke(spoke.id)">
                    <template #trigger><n-button size="tiny" type="error" secondary :disabled="!store.isAdmin">删除</n-button></template>
                    删除登记并立即撤销访问，确定继续？
                  </n-popconfirm>
                </n-space>
              </td>
            </tr>
          </tbody>
        </n-table>
      </n-scrollbar>
    </n-card>

    <template v-if="selected">
      <n-grid :cols="2" :x-gap="16" :y-gap="16" responsive="screen" item-responsive class="mb-4">
        <n-grid-item span="2 m:1">
          <n-card title="OpenNHRP 接口">
            <n-scrollbar x-scrollable><n-table size="small" :bordered="false" style="min-width: 520px">
              <thead><tr><th>名称</th><th>Protocol IP</th><th>NBMA</th><th>MTU</th></tr></thead>
              <tbody>
                <tr v-if="interfaces.length === 0"><td colspan="4" class="empty">暂无接口数据</td></tr>
                <tr v-for="item in interfaces" :key="item.name"><td><code>{{ item.name }}</code></td><td>{{ item.protocol_address || '-' }}</td><td>{{ item.nbma_address || '-' }}</td><td>{{ item.mtu || '-' }}</td></tr>
              </tbody>
            </n-table></n-scrollbar>
          </n-card>
        </n-grid-item>
        <n-grid-item span="2 m:1">
          <n-card title="当前 Hub / NHRP peers">
            <n-scrollbar x-scrollable><n-table size="small" :bordered="false" style="min-width: 520px">
              <thead><tr><th>Protocol IP</th><th>NBMA</th><th>接口</th><th>类型</th><th>租约</th></tr></thead>
              <tbody>
                <tr v-if="peers.length === 0"><td colspan="5" class="empty">暂无 peer 数据</td></tr>
                <tr v-for="peer in peers" :key="`${peer.interface}-${peer.protocol_address}`"><td>{{ peer.protocol_address }}</td><td>{{ peer.nbma_address || '-' }}</td><td>{{ peer.interface }}</td><td>{{ peer.type }}</td><td>{{ peer.expires_in_sec }}s<span v-if="peer.stale">（缓存）</span></td></tr>
              </tbody>
            </n-table></n-scrollbar>
          </n-card>
        </n-grid-item>
      </n-grid>

      <n-card title="opennhrp.conf" class="mb-4">
        <n-input v-model:value="configContent" type="textarea" :rows="14" class="config-editor" />
        <n-space justify="end" class="mt-2">
          <n-button warning secondary :disabled="!store.isAdmin" @click="reloadConfig">Reload</n-button>
          <n-button type="primary" :loading="saving" :disabled="!store.isAdmin" @click="saveConfig">保存配置</n-button>
        </n-space>
      </n-card>

      <n-card title="节点实时日志" class="log-card">
        <terminal-log :node-id="selected.id" :title="`${selected.name} / ${selected.id}`" />
      </n-card>
    </template>

    <n-modal v-model:show="showCreate" preset="card" title="登记 Spoke" style="width: 480px; max-width: calc(100vw - 32px)">
      <n-form label-placement="left" label-width="90">
        <n-form-item label="节点 ID"><n-input v-model:value="createForm.id" placeholder="branch-shanghai-01" /></n-form-item>
        <n-form-item label="显示名称"><n-input v-model:value="createForm.name" placeholder="上海分支 01" /></n-form-item>
        <n-form-item label="Protocol IP"><n-input v-model:value="createForm.protocol_address" placeholder="可选，用于关联 Hub 注册表" /></n-form-item>
      </n-form>
      <template #footer><n-space justify="end"><n-button @click="showCreate = false">取消</n-button><n-button type="primary" @click="createSpoke">创建</n-button></n-space></template>
    </n-modal>

    <n-modal v-model:show="showToken" preset="card" title="Spoke 独立令牌" style="width: 560px; max-width: calc(100vw - 32px)">
      <n-alert type="warning" class="mb-4">令牌只显示这一次。请立即写入 Agent 配置；轮换时旧连接已被断开。</n-alert>
      <n-input :value="issuedToken" readonly type="textarea" :rows="3" />
      <template #footer><n-space justify="end"><n-button @click="copyToken">复制令牌</n-button><n-button type="primary" @click="showToken = false">完成</n-button></n-space></template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { NAlert, NButton, NCard, NForm, NFormItem, NGrid, NGridItem, NInput, NModal, NPopconfirm, NScrollbar, NSpace, NTable, NTag, useMessage } from 'naive-ui'
import { api } from '../api/client'
import TerminalLog from '../components/TerminalLog.vue'
import { useAppStore } from '../store'
import type { InterfaceInfo, ManagedSpoke, SpokeInfo } from '../types'

const store = useAppStore()
const message = useMessage()
const route = useRoute()
const spokes = ref<ManagedSpoke[]>([])
const selectedId = ref('')
const interfaces = ref<InterfaceInfo[]>([])
const peers = ref<SpokeInfo[]>([])
const configContent = ref('')
const saving = ref(false)
const showCreate = ref(false)
const showToken = ref(false)
const issuedToken = ref('')
const createForm = ref({ id: '', name: '', protocol_address: '' })
const selected = computed(() => spokes.value.find((item) => item.id === selectedId.value))
let refreshTimer: number | undefined

const formatTime = (value?: string) => value ? new Date(value).toLocaleString() : '-'

const loadSpokes = async () => {
  try {
    spokes.value = await api.listManagedSpokes()
    if (selectedId.value && !spokes.value.some((item) => item.id === selectedId.value)) selectedId.value = ''
  } catch (error: any) {
    message.error(error.response?.data?.error || '加载 Spoke 失败')
  }
}

const selectSpoke = async (id: string) => {
  selectedId.value = id
  const [ifaces, peerList, config] = await Promise.allSettled([api.listInterfaces(id), api.listManagedSpokePeers(id), api.getConfigFile(id)])
  interfaces.value = ifaces.status === 'fulfilled' ? ifaces.value : []
  peers.value = peerList.status === 'fulfilled' ? peerList.value : []
  configContent.value = config.status === 'fulfilled' ? config.value.content : ''
  if (ifaces.status === 'rejected' && peerList.status === 'rejected' && config.status === 'rejected') message.error('读取 Spoke 状态失败')
}

const showIssuedToken = (token: string) => {
  issuedToken.value = token
  showToken.value = true
}

const createSpoke = async () => {
  try {
    const result = await api.createManagedSpoke(createForm.value)
    showCreate.value = false
    createForm.value = { id: '', name: '', protocol_address: '' }
    showIssuedToken(result.token)
    await loadSpokes()
  } catch (error: any) {
    message.error(error.response?.data?.error || '创建失败')
  }
}

const rotateToken = async (id: string) => {
  try {
    showIssuedToken((await api.rotateManagedSpokeToken(id)).token)
    await loadSpokes()
  } catch (error: any) {
    message.error(error.response?.data?.error || '轮换失败')
  }
}

const deleteSpoke = async (id: string) => {
  try {
    await api.deleteManagedSpoke(id)
    selectedId.value = ''
    await loadSpokes()
  } catch (error: any) {
    message.error(error.response?.data?.error || '删除失败')
  }
}

const saveConfig = async () => {
  saving.value = true
  try {
    await api.saveConfigFile(selectedId.value, { content: configContent.value, comment: 'Spoke 设备管理修改' })
    message.success('配置已原子保存，上一版本保留为 .bak')
  } catch (error: any) {
    message.error(error.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

const reloadConfig = async () => {
  try {
    await api.reloadConfig(selectedId.value)
    message.success('OpenNHRP Reload 成功')
    await selectSpoke(selectedId.value)
  } catch (error: any) {
    message.error(error.response?.data?.error || 'Reload 失败')
  }
}

const copyToken = async () => {
  await navigator.clipboard.writeText(issuedToken.value)
  message.success('令牌已复制')
}

onMounted(async () => {
  await loadSpokes()
  const requestedNode = typeof route.query.node === 'string' ? route.query.node : ''
  if (requestedNode && spokes.value.some((item) => item.id === requestedNode)) await selectSpoke(requestedNode)
  refreshTimer = window.setInterval(loadSpokes, 3000)
})
onUnmounted(() => refreshTimer && window.clearInterval(refreshTimer))
</script>

<style scoped>
.page-container { padding: 24px; }
.page-header { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 20px; }
.page-header h2 { margin: 0; font-size: 20px; color: var(--text-title); }
.sub-title, .empty { color: var(--text-muted); }
.empty { text-align: center; padding: 18px; }
.mb-4 { margin-bottom: 16px; }
.mt-2 { margin-top: 8px; }
tbody tr { cursor: pointer; }
tbody tr.selected { background: var(--bg-card-secondary); }
.config-editor { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 13px; }
.log-card { height: 520px; }
@media (max-width: 768px) {
  .page-container { padding: 12px; }
  .page-header { align-items: stretch; flex-direction: column; }
  .page-header .n-space, .page-header .n-button { width: 100%; }
}
</style>
