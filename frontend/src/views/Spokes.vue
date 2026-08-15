<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h2>Spoke 客户端与分支管理</h2>
        <span class="sub-title">全网 NHRP 动态注册表、影子复制同步、静态映射与分支状态监控</span>
      </div>
      <n-space>
        <n-button
          type="primary"
          secondary
          :disabled="!store.isAdmin"
          :title="!store.isAdmin ? '只读用户无权操作' : ''"
          @click="showAddMapModal = true"
        >
          + 添加静态映射 (Static Map)
        </n-button>
        <n-button
          type="warning"
          secondary
          :disabled="!store.isAdmin"
          :title="!store.isAdmin ? '只读用户无权操作' : ''"
          @click="handleSaveMap"
        >
          保存 Map 表 (Save)
        </n-button>
        <n-button
          type="error"
          secondary
          :disabled="!store.isAdmin"
          :title="!store.isAdmin ? '只读用户无权操作' : ''"
          @click="handlePurgeRedirect"
        >
          清除重定向缓存 (Purge)
        </n-button>
      </n-space>
    </div>

    <!-- Search & Filter Bar with Node Selector on the left -->
    <n-card size="small" class="search-card mb-4">
      <n-space justify="space-between" align="center" :wrap="true">
        <n-space align="center" :wrap="true">
          <n-select
            v-model:value="store.activeNodeId"
            :options="store.nodeOptions"
            placeholder="选择 Hub 节点"
            style="width: 240px;"
            @update:value="loadSpokes"
          />
          <n-input
            v-model:value="searchText"
            placeholder="搜索 IP 地址 / 别名 / 站点..."
            clearable
            style="width: 240px;"
          />
          <n-select
            v-model:value="selectedType"
            :options="[
              { label: '全部类型', value: 'all' },
              { label: '动态注册 (Dynamic)', value: 'dynamic' },
              { label: '静态映射 (Static)', value: 'static' },
              { label: '影子复制 (Shadow)', value: 'shadow' }
            ]"
            style="width: 170px;"
          />
        </n-space>
        <n-space align="center">
          <span class="text-muted">总计:</span>
          <span class="font-semibold text-emerald">{{ filteredSpokes.length }}</span>
          <span class="text-muted">台在线 Spoke</span>
        </n-space>
      </n-space>
    </n-card>

    <!-- Spokes Data Table -->
    <n-card>
      <n-scrollbar x-scrollable>
        <n-table :bordered="false" :single-line="true" style="min-width: 900px;">
        <thead>
          <tr>
            <th style="width: 140px;">Protocol IP (GRE)</th>
            <th style="width: 160px;">NBMA 外网物理地址</th>
            <th style="width: 150px;">NAT 映射地址</th>
            <th style="width: 110px;">隧道接口</th>
            <th style="width: 90px;">注册类型</th>
            <th style="width: 110px;">标志位 (Flags)</th>
            <th style="width: 90px;">租约剩余</th>
            <th style="min-width: 120px;">别名 / 备注</th>
            <th style="width: 340px;">纳管 / 操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading && filteredSpokes.length === 0">
            <td colspan="9" class="text-center text-muted">
              <n-skeleton text :repeat="3" style="margin: 10px 0;" />
            </td>
          </tr>
          <tr v-else-if="filteredSpokes.length === 0">
            <td colspan="9" class="text-center text-muted">
              当前节点暂无匹配的 Spoke 客户端
            </td>
          </tr>
          <tr v-for="s in filteredSpokes" :key="s.protocol_address">
            <td>
              <strong>{{ s.protocol_address }}</strong>
            </td>
            <td>
              <code>{{ s.nbma_address }}</code>
            </td>
            <td>
              <span v-if="s.nat_address"><code>{{ s.nat_address }}</code></span>
              <span v-else class="text-muted">-</span>
            </td>
            <td><code>{{ s.interface }}</code></td>
            <td>
              <n-tag
                size="small"
                :type="s.type === 'direct' || s.type === 'dynamic' ? 'success' : s.type === 'shadow' ? 'info' : 'warning'"
              >
                {{ s.type }}
              </n-tag>
            </td>
            <td>
              <span class="font-xs text-muted">{{ s.flags || 'up' }}</span>
            </td>
            <td>
              <span :class="s.expires_in_sec < 60 ? 'text-error' : ''">
                {{ s.expires_in_sec }}s
              </span>
            </td>
            <td>
              <div v-if="s.alias || s.site_name">
                <span class="font-semibold">{{ s.alias }}</span>
                <n-tag v-if="s.site_name" size="tiny" class="ml-1">{{ s.site_name }}</n-tag>
              </div>
              <span v-else class="text-muted">-</span>
            </td>
            <td>
              <n-space>
                <n-tag v-if="s.managed_node_id" size="small" :type="s.managed_status === 'online' ? 'success' : 'default'">
                  Agent {{ s.managed_status === 'online' ? '在线' : '离线' }}
                </n-tag>
                <n-button
                  v-if="s.managed_node_id"
                  size="tiny"
                  type="primary"
                  secondary
                  @click="goManagedSpoke(s.managed_node_id)"
                >
                  设备管理
                </n-button>
                <n-button
                  v-else-if="s.type !== 'local'"
                  size="tiny"
                  type="primary"
                  secondary
                  :disabled="!store.isAdmin"
                  @click="openQuickRegister(s)"
                >
                  快速登记
                </n-button>
                <n-button size="tiny" secondary :disabled="!store.isAdmin" @click="openEditMeta(s)">
                  备注
                </n-button>
                <n-button
                  v-if="s.type === 'static'"
                  size="tiny"
                  type="error"
                  secondary
                  :disabled="!store.isAdmin"
                  @click="handleDeleteMap(s)"
                >
                  删除
                </n-button>
              </n-space>
            </td>
          </tr>
        </tbody>
      </n-table>
      </n-scrollbar>
    </n-card>

    <!-- Add Static Map Modal -->
    <n-modal v-model:show="showAddMapModal" preset="card" title="添加静态 NHRP 映射 (Static Map)" style="width: 480px; max-width: calc(100vw - 32px);">
      <n-form label-placement="left" label-width="130">
        <n-form-item label="隧道接口">
          <n-input v-model:value="mapForm.interface" placeholder="如 gre4-tun0" />
        </n-form-item>
        <n-form-item label="Protocol IP">
          <n-input v-model:value="mapForm.protocol_address" placeholder="如 10.164.0.50" />
        </n-form-item>
        <n-form-item label="NBMA 物理 IP">
          <n-input v-model:value="mapForm.nbma_address" placeholder="如 198.51.100.2" />
        </n-form-item>
        <n-form-item label="自动注册">
          <n-switch v-model:value="mapForm.register" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showAddMapModal = false">取消</n-button>
          <n-button type="primary" @click="handleAddMap">保存映射</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Edit Metadata Modal -->
    <n-modal v-model:show="showMetaModal" preset="card" title="编辑 Spoke 站点备注" style="width: 440px; max-width: calc(100vw - 32px);">
      <n-form label-placement="left" label-width="100">
        <n-form-item label="Protocol IP">
          <n-input :value="metaForm.protocol_address" disabled />
        </n-form-item>
        <n-form-item label="设备别名">
          <n-input v-model:value="metaForm.alias" placeholder="如 上海分支-01" />
        </n-form-item>
        <n-form-item label="所属站点">
          <n-input v-model:value="metaForm.site_name" placeholder="如 华东数据中心" />
        </n-form-item>
        <n-form-item label="备注信息">
          <n-input type="textarea" v-model:value="metaForm.notes" placeholder="可选备注..." />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showMetaModal = false">取消</n-button>
          <n-button type="primary" @click="handleSaveMeta">保存备注</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="showQuickRegister" preset="card" title="快速登记 Spoke Agent" style="width: 500px; max-width: calc(100vw - 32px);">
      <n-alert type="info" class="mb-4">将 Hub 观察记录绑定到同一台 Managed Spoke，并生成独立的一次性 Token。</n-alert>
      <n-form label-placement="left" label-width="110">
        <n-form-item label="Protocol IP"><n-input :value="quickSource.protocol_address" disabled /></n-form-item>
        <n-form-item label="NBMA 地址"><n-input :value="quickSource.nbma_address" disabled /></n-form-item>
        <n-form-item label="纳管设备"><n-select v-model:value="quickTarget" :options="quickTargetOptions" /></n-form-item>
        <template v-if="quickTarget === '__new__'">
          <n-form-item label="Agent 节点 ID"><n-input v-model:value="quickForm.id" /></n-form-item>
          <n-form-item label="显示名称"><n-input v-model:value="quickForm.name" /></n-form-item>
        </template>
        <n-alert v-else type="warning">关联已有设备会轮换其 Token，并立即断开旧 Agent 连接。</n-alert>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showQuickRegister = false">取消</n-button>
          <n-button type="primary" :loading="registering" @click="handleQuickRegister">绑定并生成 Token</n-button>
        </n-space>
      </template>
    </n-modal>

    <n-modal v-model:show="showToken" preset="card" title="Spoke Agent 一次性 Token" style="width: 600px; max-width: calc(100vw - 32px);">
      <n-alert type="warning" class="mb-4">Token 只显示这一次，请立即写入对应 Spoke 的 Agent 配置。</n-alert>
      <n-form label-placement="left" label-width="110">
        <n-form-item label="Manager WS"><n-input :value="managerWSURL" readonly /></n-form-item>
        <n-form-item label="节点类型"><n-input value="spoke" readonly /></n-form-item>
        <n-form-item label="节点 ID"><n-input :value="issuedNodeID" readonly /></n-form-item>
        <n-form-item label="Token"><n-input :value="issuedToken" type="textarea" :rows="3" readonly /></n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="copyAgentSettings">复制 Agent 参数</n-button>
          <n-button type="primary" @click="showToken = false">完成</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  NCard,
  NButton,
  NInput,
  NSelect,
  NTable,
  NTag,
  NSpace,
  NScrollbar,
  NModal,
  NForm,
  NFormItem,
  NSwitch,
  NSkeleton,
  NAlert,
  useMessage,
} from 'naive-ui'
import { api } from '../api/client'
import { useAppStore } from '../store'
import { sortSpokesByIP } from '../utils/ip'
import type { ManagedSpoke, SpokeInfo } from '../types'

const store = useAppStore()
const message = useMessage()
const router = useRouter()

const loading = ref(true)
const spokes = ref<SpokeInfo[]>([])
const searchText = ref('')
const selectedType = ref('all')

const showAddMapModal = ref(false)
const showMetaModal = ref(false)
const showQuickRegister = ref(false)
const showToken = ref(false)
const registering = ref(false)
const quickSource = ref({ protocol_address: '', nbma_address: '' })
const quickForm = ref({ id: '', name: '', protocol_address: '' })
const quickTarget = ref('__new__')
const managedSpokeChoices = ref<ManagedSpoke[]>([])
const quickTargetOptions = computed(() => [
  { label: '新登记 Managed Spoke', value: '__new__' },
  ...managedSpokeChoices.value.map((item) => ({ label: `关联已有：${item.name} (${item.id})`, value: item.id })),
])
const issuedNodeID = ref('')
const issuedToken = ref('')
const managerWSURL = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/agent/ws`

const mapForm = ref({
  interface: 'gre4-tun0',
  protocol_address: '',
  nbma_address: '',
  register: true,
})

const metaForm = ref({
  protocol_address: '',
  alias: '',
  site_name: '',
  notes: '',
})

const filteredSpokes = computed(() => {
  const filtered = spokes.value.filter((s) => {
    const matchSearch =
      !searchText.value ||
      s.protocol_address.includes(searchText.value) ||
      s.nbma_address.includes(searchText.value) ||
      (s.alias && s.alias.includes(searchText.value)) ||
      (s.site_name && s.site_name.includes(searchText.value))

    const matchType = selectedType.value === 'all' || s.type === selectedType.value
    return matchSearch && matchType
  })
  return sortSpokesByIP(filtered)
})

const loadSpokes = async () => {
  try {
    const targetNode = store.activeNodeId
    const res = await api.listSpokes(targetNode)
    spokes.value = sortSpokesByIP(res)
  } catch (e) {
    console.error('Failed to list spokes', e)
  } finally {
    loading.value = false
  }
}

watch(
  () => store.activeNodeId,
  () => {
    loadSpokes()
  }
)

const handleAddMap = async () => {
  if (!mapForm.value.protocol_address || !mapForm.value.nbma_address) {
    message.error('请填写完整的 IP 地址')
    return
  }
  try {
    const targetNode = store.activeNodeId
    await api.addStaticMap(targetNode, mapForm.value)
    message.success('静态 Map 添加成功')
    showAddMapModal.value = false
    loadSpokes()
  } catch (e: any) {
    message.error(e.response?.data?.error || '添加失败')
  }
}

const handleDeleteMap = async (s: SpokeInfo) => {
  try {
    const targetNode = store.activeNodeId
    await api.delStaticMap(targetNode, {
      interface: s.interface,
      protocol_address: s.protocol_address,
    })
    message.success(`已删除 ${s.protocol_address}`)
    loadSpokes()
  } catch (e: any) {
    message.error('删除失败')
  }
}

const handleSaveMap = async () => {
  try {
    const targetNode = store.activeNodeId
    await api.saveMap(targetNode, mapForm.value.interface)
    message.success('已持久化保存当前 Map 表')
  } catch (e: any) {
    message.error('保存失败')
  }
}

const handlePurgeRedirect = async () => {
  try {
    const targetNode = store.activeNodeId
    await api.purgeRedirect(targetNode)
    message.success('已清除全部重定向与限流缓存')
  } catch (e: any) {
    message.error('清理失败')
  }
}

const openEditMeta = (s: SpokeInfo) => {
  metaForm.value = {
    protocol_address: s.protocol_address,
    alias: s.alias || '',
    site_name: s.site_name || '',
    notes: '',
  }
  showMetaModal.value = true
}

const handleSaveMeta = async () => {
  try {
    await api.setSpokeMetadata(metaForm.value)
    message.success('备注保存成功')
    showMetaModal.value = false
    loadSpokes()
  } catch (e: any) {
    message.error('保存失败')
  }
}

const openQuickRegister = async (s: SpokeInfo) => {
  const protocolIP = s.protocol_address.replace(/\/.*$/, '')
  quickSource.value = { protocol_address: s.protocol_address, nbma_address: s.nbma_address }
  quickForm.value = {
    id: `spoke-${protocolIP.replace(/[^A-Za-z0-9._-]/g, '-')}`.slice(0, 64),
    name: (s.alias || s.site_name || `Spoke ${protocolIP}`).slice(0, 128),
    protocol_address: s.protocol_address,
  }
  managedSpokeChoices.value = []
  quickTarget.value = '__new__'
  showQuickRegister.value = true
  try {
    managedSpokeChoices.value = (await api.listManagedSpokes()).filter((item) => !item.protocol_address)
    if (managedSpokeChoices.value.length === 1) quickTarget.value = managedSpokeChoices.value[0].id
  } catch (e: any) {
    message.warning(e.response?.data?.error || '未能加载已有 Managed Spoke，可继续新登记')
  }
}

const handleQuickRegister = async () => {
  registering.value = true
  try {
    if (quickTarget.value === '__new__') {
      const result = await api.createManagedSpoke(quickForm.value)
      issuedNodeID.value = result.spoke.id
      issuedToken.value = result.token
    } else {
      issuedNodeID.value = quickTarget.value
      issuedToken.value = (await api.rotateManagedSpokeToken(quickTarget.value, quickSource.value.protocol_address)).token
    }
    showQuickRegister.value = false
    showToken.value = true
    await loadSpokes()
  } catch (e: any) {
    message.error(e.response?.data?.error || '登记失败')
  } finally {
    registering.value = false
  }
}

const goManagedSpoke = (id: string) => router.push({ path: '/managed-spokes', query: { node: id } })

const copyAgentSettings = async () => {
  await navigator.clipboard.writeText(`SERVER=${managerWSURL}\nNODE_ID=${issuedNodeID.value}\nNODE_TYPE=spoke\nTOKEN=${issuedToken.value}`)
  message.success('Agent 参数已复制')
}

onMounted(loadSpokes)
</script>

<style scoped>
.page-container {
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
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

.search-card {
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  box-shadow: var(--card-shadow);
  transition: background-color 0.2s ease, border-color 0.2s ease;
}

.text-muted { color: var(--text-muted); }
.text-emerald { color: #10b981; }
.text-error { color: #ef4444; }
.font-semibold { font-weight: 600; }
.font-xs { font-size: 11px; }
.mb-4 { margin-bottom: 16px; }
.ml-1 { margin-left: 4px; }
.text-center { text-align: center; }

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
  .page-header .n-button {
    width: 100% !important;
    justify-content: center !important;
  }
  .search-card .n-space {
    width: 100%;
    flex-direction: column !important;
    align-items: stretch !important;
    gap: 8px !important;
  }
  .search-card .n-select,
  .search-card .n-input {
    width: 100% !important;
  }
}
</style>
