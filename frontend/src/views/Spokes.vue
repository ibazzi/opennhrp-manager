<template>
  <div class="spoke-panel">
    <div class="page-header">
      <span class="sub-title">汇总所有 Hub 的 NHRP 接入记录，并统一管理已登记的 Spoke Agent</span>
      <n-button type="primary" :disabled="!store.isAdmin" @click="openCreate">登记 Spoke</n-button>
    </div>
    <n-alert v-if="loadFailures.length" type="warning" class="mb-4">以下 Hub 暂时无法读取，其他数据仍正常显示：{{ loadFailures.join('、') }}</n-alert>
    <n-card size="small" class="search-card mb-4">
      <n-space justify="space-between" align="center" :wrap="true">
        <n-space align="center" :wrap="true">
          <n-input v-model:value="searchText" placeholder="搜索 Hub / Spoke / IP / 别名..." clearable style="width: 300px" />
          <n-select v-model:value="selectedType" :options="typeOptions" style="width: 180px" />
        </n-space>
        <span class="text-muted">共 <strong class="text-emerald">{{ filteredRows.length }}</strong> 条，覆盖 {{ hubCount }} 个 Hub</span>
      </n-space>
    </n-card>

    <n-card class="table-card">
      <n-scrollbar x-scrollable class="table-scroll" style="height: 100%; max-height: none">
        <n-table :bordered="false" :single-line="true" style="min-width: 1320px">
          <thead><tr><th style="width: 120px">Protocol IP</th><th style="width: 110px">所属 Hub</th><th>Spoke / 备注</th><th style="width: 120px">接入信息</th><th style="width: 110px">运行状态</th><th style="width: 140px">Agent 遥测</th><th style="width: 160px">最后心跳</th><th style="width: 150px">操作</th></tr></thead>
          <tbody>
            <tr v-if="loading && filteredRows.length === 0"><td colspan="8"><n-skeleton text :repeat="3" /></td></tr>
            <tr v-else-if="filteredRows.length === 0"><td colspan="8" class="empty">暂无匹配的 Spoke</td></tr>
            <tr v-for="row in filteredRows" :key="rowKey(row)" :class="{ manageable: !!row.managed }" @click="row.managed && openManaged(row.managed.id)">
              <td class="address-cell">{{ row.spoke?.protocol_address || row.managed?.protocol_address || '-' }}</td>
              <td class="address-cell"><span v-if="row.hubId">{{ row.hubMemberId }}</span><span v-else class="text-muted">未接入</span></td>
              <td><template v-if="row.managed"><strong>{{ row.managed.name }}</strong> <code>{{ row.managed.id }}</code></template><template v-else-if="row.spoke?.alias"><strong>{{ row.spoke.alias }}</strong></template><span v-else class="text-muted">-</span></td>
              <td><span v-if="row.spoke" class="access-tags"><n-tag size="tiny" :type="row.spoke.type === 'shadow' ? 'info' : 'success'">{{ row.spoke.type }}</n-tag><n-tag v-if="row.spoke.registration_mode" size="tiny" type="info">{{ row.spoke.registration_mode === 'ha' ? 'HA' : row.spoke.registration_mode }}</n-tag></span><span v-else class="text-muted">-</span></td>
              <td><n-tag v-if="row.managed" size="tiny" :type="row.managed.status === 'online' ? 'success' : row.managed.status === 'degraded' ? 'warning' : 'default'">Agent {{ statusText(row.managed.status) }}</n-tag><n-tag v-else size="tiny">未纳管</n-tag></td>
              <td><span v-if="row.managed">Peers {{ row.managed.peer_count }} · {{ row.managed.ws_rtt_ms ? `${row.managed.ws_rtt_ms.toFixed(1)} ms` : '-' }}</span><span v-else class="text-muted">-</span></td>
              <td><span v-if="row.managed">{{ formatTime(row.managed.last_seen) }}</span><span v-else class="text-muted">-</span></td>
              <td @click.stop>
                <n-space>
                  <n-button v-if="row.spoke || row.managed" size="tiny" secondary :disabled="!store.isAdmin" @click="openEditMeta(row)">备注</n-button>
                  <n-button v-if="!row.managed && row.spoke?.type !== 'local'" size="tiny" type="primary" secondary :disabled="!store.isAdmin" @click="openQuickRegister(row.spoke!)">快速登记</n-button>
                  <n-popconfirm v-if="row.spoke?.type === 'static'" :disabled="!store.isAdmin" @positive-click="deleteMap(row)"><template #trigger><n-button size="tiny" type="error" secondary :disabled="!store.isAdmin">删除映射</n-button></template>确认从 {{ row.hubMemberId }} 删除这条静态映射？</n-popconfirm>
                  <span v-if="row.managed && !row.spoke" class="text-muted">点击行管理</span>
                </n-space>
              </td>
            </tr>
          </tbody>
        </n-table>
      </n-scrollbar>
    </n-card>

    <n-modal :show="showManage" display-directive="if" preset="card" size="small" :title="manageTitle" class="manage-modal" @update:show="handleManageVisibility">
      <template #header-extra><n-space @click.stop><n-popconfirm :disabled="!store.isAdmin" @positive-click="rotateToken(selectedManagedId)"><template #trigger><n-button size="small" warning secondary :disabled="!store.isAdmin">轮换令牌</n-button></template>轮换后当前 Agent 会立即断开，确定继续？</n-popconfirm><n-popconfirm :disabled="!store.isAdmin" @positive-click="deleteManagedSpoke"><template #trigger><n-button size="small" type="error" secondary :disabled="!store.isAdmin">删除登记</n-button></template>删除登记并立即撤销访问，确定继续？</n-popconfirm></n-space></template>
      <n-spin :show="detailLoading">
        <n-grid :cols="1" :y-gap="12" class="mb-4">
          <n-grid-item><n-card size="small" title="OpenNHRP 接口" class="peer-summary-card"><n-table size="small" :bordered="false"><thead><tr><th>名称</th><th style="width: 180px">Protocol IP</th><th style="width: 180px">NBMA</th><th style="width: 100px">MTU</th></tr></thead><tbody><tr v-if="interfaces.length === 0"><td colspan="4" class="empty">暂无接口数据</td></tr><tr v-for="item in interfaces" :key="item.name"><td><code>{{ item.name }}</code></td><td>{{ item.protocol_address || '-' }}</td><td>{{ item.nbma_address || '-' }}</td><td>{{ item.mtu || '-' }}</td></tr></tbody></n-table></n-card></n-grid-item>
          <n-grid-item><n-card size="small" title="当前 Hub / NHRP peers" class="peer-summary-card"><n-table size="small" :bordered="false"><thead><tr><th style="width: 180px">Protocol IP</th><th>NBMA</th><th style="width: 120px">接口</th><th style="width: 120px">类型</th><th style="width: 120px">租约</th></tr></thead><tbody><tr v-if="peers.length === 0"><td colspan="5" class="empty">暂无 peer 数据</td></tr><tr v-for="peer in peers" :key="`${peer.interface}-${peer.protocol_address}`"><td>{{ peer.protocol_address }}</td><td>{{ peer.nbma_address || '-' }}</td><td>{{ peer.interface }}</td><td>{{ peer.type }}</td><td>{{ peer.expires_in_sec }}s<span v-if="peer.stale">（缓存）</span></td></tr></tbody></n-table></n-card></n-grid-item>
        </n-grid>

        <n-card v-if="haStatus" size="small" title="Spoke HA Hub 路径与质量" class="mb-4">
          <div class="ha-summary"><span>接口: <code>{{ haStatus.interface }}</code></span><span>当前 Hub: <code>{{ haStatus.active_member || '-' }}</code></span><span>选择模式: <n-tag size="small" :type="haStatus.selection_mode === 'manual' ? 'warning' : 'info'">{{ haStatus.selection_mode === 'manual' ? (haStatus.manual_suspended ? '手动（HA 接管中）' : '手动') : '自动' }}</n-tag></span><span v-if="haStatus.selection_mode === 'manual'">手动目标: <code>{{ haStatus.manual_member }}</code></span><span>协调器: {{ haStatus.coordinator_state }}</span><span>切换: {{ haStatus.switching ? '进行中' : '稳定' }}</span><n-popconfirm v-if="haStatus.selection_mode === 'manual'" :disabled="!store.isAdmin || !!modeChanging" @positive-click="setHAMode('auto')"><template #trigger><n-button size="tiny" secondary :loading="modeChanging === 'auto'" :disabled="!store.isAdmin || !!modeChanging">恢复自动</n-button></template>清除手动目标并恢复自动评分切换策略？</n-popconfirm></div>
          <p>失败率统计最近 30 秒内已完成的探测，包含超时和无效回复。评分 RTT 独立平滑；SRTT 用于超时估计。</p>
          <n-table size="small" :bordered="false"><thead><tr><th style="width: 150px">Hub</th><th style="width: 100px">状态</th><th style="width: 150px">Selected endpoint</th><th style="width: 150px">评分 RTT</th><th style="width: 120px">探测失败率</th><th style="width: 180px">有效总分</th><th>Term / Leader</th><th style="width: 130px">操作</th></tr></thead><tbody>
            <tr v-if="haStatus.candidates.length === 0"><td colspan="8" class="empty">暂无 Hub 候选</td></tr>
            <tr v-for="candidate in haStatus.candidates" :key="candidate.member"><td><strong>{{ candidate.member }}</strong> <n-tag v-if="candidate.active" size="tiny" type="success">当前</n-tag> <n-tag v-if="haStatus.manual_member === candidate.member" size="tiny" type="warning">手动目标</n-tag></td><td><n-tag size="small" :type="candidate.ready ? 'success' : candidate.state === 'suspect' ? 'warning' : 'default'">{{ candidate.state }}</n-tag></td><td><code>{{ candidate.selected_address || '-' }}</code></td><td><div>{{ candidate.quality_rtt_ms == null ? '—' : candidate.quality_rtt_ms.toFixed(1) + ' ms' }}</div><small>超时估计 SRTT：{{ candidate.srtt_ms.toFixed(1) }} ms</small><br><small>有效回复距今：{{ candidate.last_quality_reply_age_ms == null ? '—' : (candidate.last_quality_reply_age_ms / 1000).toFixed(1) + ' s' }}</small></td><td><div>{{ candidate.quality_samples > 0 ? candidate.loss_pct.toFixed(1) + '%' : '—' }}</div><small>30 秒：{{ candidate.quality_failures ?? 0 }} / {{ candidate.quality_samples ?? 0 }} 次失败</small></td><td><n-tag size="small" :type="candidate.active ? 'success' : 'info'">{{ candidate.score }}</n-tag> <n-tag v-if="!candidate.quality_valid" size="tiny" type="warning">测量不足或已过期</n-tag><div><small>失败率 / 延时 / 优先级：{{ candidate.loss_score?.toFixed(2) ?? '—' }} / {{ candidate.latency_score?.toFixed(2) ?? '—' }} / {{ candidate.priority_score?.toFixed(2) ?? '—' }}</small></div></td><td>{{ candidate.term }} / {{ candidate.leader || '-' }}</td><td><span v-if="candidate.active || haStatus.manual_member === candidate.member">-</span><n-popconfirm v-else :disabled="!store.isAdmin || !!modeChanging || !!haStatus.switching || !candidate.ready || !candidate.authenticated" @positive-click="setHAMode('manual', candidate.member)"><template #trigger><n-button size="tiny" secondary type="warning" :loading="modeChanging === candidate.member" :disabled="!store.isAdmin || !!modeChanging || !!haStatus.switching || !candidate.ready || !candidate.authenticated">设为手动目标</n-button></template>将当前 Spoke 手动切换到 {{ candidate.member }}？</n-popconfirm></td></tr>
          </tbody></n-table>
        </n-card>

      </n-spin>
    </n-modal>

    <n-modal v-model:show="showMetaModal" preset="card" title="编辑 Spoke 信息" style="width: 440px; max-width: calc(100vw - 32px)"><n-form label-placement="left" label-width="100"><n-form-item label="Protocol IP"><n-input :value="metaForm.protocol_address" disabled /></n-form-item><n-form-item label="设备别名"><n-input v-model:value="metaForm.alias" /></n-form-item><n-form-item label="备注信息"><n-input v-model:value="metaForm.notes" type="textarea" :rows="3" /></n-form-item></n-form><template #footer><n-space justify="end"><n-button @click="showMetaModal = false">取消</n-button><n-button type="primary" @click="saveMeta">保存</n-button></n-space></template></n-modal>
    <n-modal v-model:show="showRegister" preset="card" :title="registerSource ? '快速登记 Spoke Agent' : '登记 Spoke'" style="width: 520px; max-width: calc(100vw - 32px)"><n-form label-placement="left" label-width="130"><template v-if="registerSource"><n-form-item label="Protocol IP"><n-input :value="registerSource.protocol_address" disabled /></n-form-item><n-form-item label="NBMA 地址"><n-input :value="registerSource.nbma_address" disabled /></n-form-item><n-form-item label="纳管设备"><n-select v-model:value="registerTarget" :options="registerOptions" /></n-form-item></template><template v-if="!registerSource || registerTarget === '__new__'"><n-form-item label="Agent 节点 ID" required><n-input v-model:value="registerForm.id" /></n-form-item><n-form-item label="显示名称" required><n-input v-model:value="registerForm.name" /></n-form-item><n-form-item v-if="!registerSource" label="Protocol IP"><n-input v-model:value="registerForm.protocol_address" placeholder="Agent 自动获取，也可预填" /></n-form-item></template><n-alert v-else type="warning">关联已有设备会轮换其 Token，并立即断开旧 Agent 连接。</n-alert></n-form><template #footer><n-space justify="end"><n-button @click="showRegister = false">取消</n-button><n-button type="primary" :loading="registering" @click="registerSpoke">登记并生成 Token</n-button></n-space></template></n-modal>
    <n-modal v-model:show="showToken" preset="card" title="Spoke Agent 一次性 Token" style="width: 600px; max-width: calc(100vw - 32px)"><n-alert type="warning" class="mb-4">Token 只显示这一次，请立即写入对应 Spoke 的 Agent 配置。</n-alert><n-form label-placement="left" label-width="110"><n-form-item label="Manager WS"><n-input :value="managerWSURL" readonly /></n-form-item><n-form-item label="节点 ID"><n-input :value="issuedNodeID" readonly /></n-form-item><n-form-item label="Token"><n-input :value="issuedToken" type="textarea" :rows="3" readonly /></n-form-item></n-form><template #footer><n-space justify="end"><n-button @click="copyAgentSettings">复制 Agent 参数</n-button><n-button type="primary" @click="showToken = false">完成</n-button></n-space></template></n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NAlert, NButton, NCard, NForm, NFormItem, NGrid, NGridItem, NInput, NModal, NPopconfirm, NScrollbar, NSelect, NSkeleton, NSpace, NSpin, NTable, NTag, useMessage } from 'naive-ui'
import { api } from '../api/client'
import { useAppStore } from '../store'
import { isHubNode } from '../utils/topologyStatus'
import type { HAStatus, InterfaceInfo, ManagedSpoke, NodeRecord, SpokeInfo, TopologySnapshot } from '../types'

interface SpokeRow { hubId: string; hubMemberId: string; spoke?: SpokeInfo; managed?: ManagedSpoke }

const store = useAppStore()
const message = useMessage()
const route = useRoute()
const router = useRouter()
const loading = ref(true)
const hubSpokes = ref<Record<string, SpokeInfo[]>>({})
const managedSpokes = ref<ManagedSpoke[]>([])
const loadFailures = ref<string[]>([])
const searchText = ref('')
const selectedType = ref('all')
const typeOptions = [{ label: '全部类型', value: 'all' }, { label: '动态注册 (Dynamic)', value: 'dynamic' }, { label: '静态映射 (Static)', value: 'static' }, { label: '影子复制 (Shadow)', value: 'shadow' }]
const showMetaModal = ref(false)
const metaForm = ref({ protocol_address: '', alias: '', notes: '' })
const editingManagedId = ref('')
const showRegister = ref(false)
const registerSource = ref<SpokeInfo | null>(null)
const registerTarget = ref('__new__')
const registerForm = ref({ id: '', name: '', protocol_address: '' })
const registering = ref(false)
const showToken = ref(false)
const issuedNodeID = ref('')
const issuedToken = ref('')
const managerWSURL = `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/api/agent/ws`
const showManage = ref(false)
const selectedManagedId = ref('')
const detailLoading = ref(false)
const interfaces = ref<InterfaceInfo[]>([])
const peers = ref<SpokeInfo[]>([])
const haStatus = ref<HAStatus | null>(null)
const modeChanging = ref('')
let haRefreshTimer: number | null = null
let haRefreshInFlight = false

const hubs = computed(() => store.nodes.filter(isHubNode))
const hubCount = computed(() => hubs.value.length)
const managedById = computed(() => new Map(managedSpokes.value.map((item) => [item.id, item])))
const managedByAddress = computed(() => new Map(managedSpokes.value.filter((item) => item.protocol_address).map((item) => [item.protocol_address!.split('/')[0], item])))
const selectedManaged = computed(() => managedById.value.get(selectedManagedId.value))
const statusText = (status: ManagedSpoke['status']) => status === 'online' ? '在线' : status === 'degraded' ? '降级' : '离线'
const manageTitle = computed(() => selectedManaged.value ? `${selectedManaged.value.name} / ${selectedManaged.value.id} · ${statusText(selectedManaged.value.status)}` : 'Spoke 管理')
const registerOptions = computed(() => [{ label: '新登记 Managed Spoke', value: '__new__' }, ...managedSpokes.value.filter((item) => !item.protocol_address).map((item) => ({ label: `关联已有：${item.name} (${item.id})`, value: item.id }))])
const rows = computed<SpokeRow[]>(() => {
  const result: SpokeRow[] = []
  const matched = new Set<string>()
  for (const hub of hubs.value) for (const spoke of hubSpokes.value[hub.id] || []) {
    const managed = (spoke.managed_node_id ? managedById.value.get(spoke.managed_node_id) : undefined) || managedByAddress.value.get(spoke.protocol_address.split('/')[0])
    if (managed) matched.add(managed.id)
    result.push({ hubId: hub.id, hubMemberId: hub.name || hub.id, spoke, managed })
  }
  for (const managed of managedSpokes.value) if (!matched.has(managed.id)) result.push({ hubId: '', hubMemberId: '', managed })
  return result
})
const filteredRows = computed(() => {
  const needle = searchText.value.trim().toLowerCase()
  return rows.value.filter((row) => {
    if (selectedType.value !== 'all' && row.spoke?.type !== selectedType.value) return false
    if (!needle) return true
    return [row.hubId, row.hubMemberId, row.managed?.id, row.managed?.name, row.spoke?.protocol_address, row.spoke?.nbma_address, row.spoke?.alias].some((value) => value?.toLowerCase().includes(needle))
  })
})
const formatTime = (value?: string) => value ? new Date(value).toLocaleString() : '-'
const rowKey = (row: SpokeRow) => row.spoke ? `${row.hubId}-${row.spoke.interface}-${row.spoke.protocol_address}-${row.spoke.type}` : `managed-${row.managed?.id}`

const managedFromNode = (node: NodeRecord): ManagedSpoke => ({
  id: node.id,
  name: node.name || node.id,
  status: node.status,
  host: node.host,
  ws_rtt_ms: node.ws_rtt_ms || 0,
  core_available: Boolean(node.service_avail),
  peer_count: node.peer_count ?? node.active_spokes ?? 0,
  last_seen: node.last_seen,
  protocol_address: node.advertised_ip || '',
})
const applyTopology = (snapshot: TopologySnapshot | null) => {
  if (!snapshot) return
  hubSpokes.value = snapshot.spokes_by_node || {}
  managedSpokes.value = snapshot.nodes.filter((node) => node.type === 'spoke').map(managedFromNode)
  loadFailures.value = snapshot.spoke_failures || []
  loading.value = false
}

const openEditMeta = (row: SpokeRow) => { editingManagedId.value = row.managed?.id || ''; metaForm.value = { protocol_address: row.spoke?.protocol_address || row.managed?.protocol_address || '', alias: row.managed?.name || row.spoke?.alias || '', notes: row.spoke?.notes || '' }; showMetaModal.value = true }
const saveMeta = async () => {
  try {
    if (editingManagedId.value) {
      await api.updateManagedSpokeName(editingManagedId.value, metaForm.value.alias)
      const managed = managedSpokes.value.find((item) => item.id === editingManagedId.value)
      if (managed) managed.name = metaForm.value.alias.trim()
      if (metaForm.value.protocol_address) await api.setSpokeMetadata({ protocol_address: metaForm.value.protocol_address, notes: metaForm.value.notes })
    } else {
      await api.setSpokeMetadata(metaForm.value)
    }
    message.success('保存成功')
    showMetaModal.value = false
  } catch (e: any) { message.error(e.response?.data?.error || '保存失败') }
}
const deleteMap = async (row: SpokeRow) => { if (!row.spoke) return; try { await api.delStaticMap(row.hubId, { interface: row.spoke.interface, protocol_address: row.spoke.protocol_address }); message.success(`已删除 ${row.spoke.protocol_address}`) } catch (e: any) { message.error(e.response?.data?.error || '删除失败') } }
const openCreate = () => { registerSource.value = null; registerTarget.value = '__new__'; registerForm.value = { id: '', name: '', protocol_address: '' }; showRegister.value = true }
const openQuickRegister = (spoke: SpokeInfo) => { const ip = spoke.protocol_address.replace(/\/.*$/, ''); registerSource.value = spoke; registerTarget.value = '__new__'; registerForm.value = { id: `spoke-${ip.replace(/[^A-Za-z0-9._-]/g, '-')}`.slice(0, 64), name: (spoke.alias || spoke.site_name || `Spoke ${ip}`).slice(0, 128), protocol_address: spoke.protocol_address }; const choices = managedSpokes.value.filter((item) => !item.protocol_address); if (choices.length === 1) registerTarget.value = choices[0].id; showRegister.value = true }
const validRegistration = () => { if (!/^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/.test(registerForm.value.id.trim())) { message.error('节点 ID 必须为 1–64 个字符，以英文字母或数字开头，仅允许英文字母、数字、点、下划线和连字符'); return false }; const length = [...registerForm.value.name.trim()].length; if (!length || length > 128) { message.error('显示名称不能为空，且不能超过 128 个字符'); return false }; return true }
const registerSpoke = async () => { registering.value = true; try { if (!registerSource.value || registerTarget.value === '__new__') { if (!validRegistration()) return; const result = await api.createManagedSpoke(registerForm.value); issuedNodeID.value = result.spoke.id; issuedToken.value = result.token } else { issuedNodeID.value = registerTarget.value; issuedToken.value = (await api.rotateManagedSpokeToken(registerTarget.value, registerSource.value.protocol_address)).token }; showRegister.value = false; showToken.value = true } catch (e: any) { message.error(e.response?.data?.error || '登记失败') } finally { registering.value = false } }
const copyAgentSettings = async () => { await navigator.clipboard.writeText(`SERVER=${managerWSURL}\nNODE_ID=${issuedNodeID.value}\nNODE_TYPE=spoke\nTOKEN=${issuedToken.value}`); message.success('Agent 参数已复制') }

const clearDetails = () => { selectedManagedId.value = ''; interfaces.value = []; peers.value = []; haStatus.value = null }
const loadHAStatus = async () => { const managedId = selectedManagedId.value; const iface = interfaces.value.find((item) => item.name === 'gre-ha')?.name || interfaces.value[0]?.name || ''; if (!iface || !managedId) { haStatus.value = null; return }; try { const status = await api.getManagedSpokeHA(managedId, iface); if (showManage.value && selectedManagedId.value === managedId) haStatus.value = status } catch { if (showManage.value && selectedManagedId.value === managedId) haStatus.value = null } }
const refreshOpenHAStatus = async () => { if (!showManage.value || detailLoading.value || !selectedManagedId.value || haRefreshInFlight) return; haRefreshInFlight = true; try { await loadHAStatus() } finally { haRefreshInFlight = false } }
const startHARefresh = () => { if (haRefreshTimer !== null) return; haRefreshTimer = window.setInterval(() => { void refreshOpenHAStatus() }, 3000) }
const stopHARefresh = () => { if (haRefreshTimer === null) return; window.clearInterval(haRefreshTimer); haRefreshTimer = null }
const loadDetails = async (id: string) => { detailLoading.value = true; selectedManagedId.value = id; haStatus.value = null; const [ifaces, peerList] = await Promise.allSettled([api.listInterfaces(id), api.listManagedSpokePeers(id)]); if (selectedManagedId.value !== id) return; interfaces.value = ifaces.status === 'fulfilled' ? ifaces.value : []; peers.value = peerList.status === 'fulfilled' ? peerList.value : []; if (ifaces.status === 'rejected' && peerList.status === 'rejected') message.error('读取 Spoke 状态失败'); await loadHAStatus(); detailLoading.value = false }
const openManaged = (id: string) => { if (route.query.node === id && showManage.value) return; router.push({ path: '/spokes', query: { ...route.query, tab: undefined, node: id } }) }
const handleManageVisibility = (visible: boolean) => { if (!visible) router.push({ path: '/spokes', query: { ...route.query, tab: undefined, node: undefined } }) }
const setHAMode = async (mode: 'auto' | 'manual', member = '') => { modeChanging.value = mode === 'auto' ? 'auto' : member; try { await api.setManagedSpokeHAMode(selectedManagedId.value, mode, member); message.success(mode === 'auto' ? '已恢复自动选择' : `已将 ${member} 设为手动目标`); await loadHAStatus(); try { peers.value = await api.listManagedSpokePeers(selectedManagedId.value) } catch {} } catch (e: any) { message.error(e.response?.data?.error || '切换 Hub 模式失败') } finally { modeChanging.value = '' } }
const rotateToken = async (id: string) => { try { issuedNodeID.value = id; issuedToken.value = (await api.rotateManagedSpokeToken(id)).token; showToken.value = true } catch (e: any) { message.error(e.response?.data?.error || '轮换失败') } }
const deleteManagedSpoke = async () => { try { await api.deleteManagedSpoke(selectedManagedId.value); message.success('已删除 Spoke 登记'); handleManageVisibility(false) } catch (e: any) { message.error(e.response?.data?.error || '删除失败') } }
const syncManagedRoute = async () => { const id = typeof route.query.node === 'string' ? route.query.node : ''; if (!id) { showManage.value = false; clearDetails(); return }; if (!managedById.value.has(id)) return; if (showManage.value && selectedManagedId.value === id) return; showManage.value = true; await loadDetails(id) }
watch(() => store.topologySnapshot, applyTopology, { immediate: true })
watch(showManage, (visible) => visible ? startHARefresh() : stopHARefresh())
watch(() => route.query.node, syncManagedRoute, { immediate: true })
watch(() => managedSpokes.value.map((item) => item.id).join(','), syncManagedRoute)
onBeforeUnmount(stopHARefresh)
</script>

<style scoped>
.page-header { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 16px; }
.sub-title, .text-muted, .empty { color: var(--text-muted); }
.text-emerald { color: #10b981; }
.text-error { color: #ef4444; }
.empty { text-align: center; padding: 18px; }
.mb-4 { margin-bottom: 16px; }
.search-card { background: var(--bg-card); border: 1px solid var(--border-color); box-shadow: var(--card-shadow); }
.spoke-panel { display: flex; flex: 1; flex-direction: column; min-height: 0; overflow: hidden; }
.table-card { flex: 1; min-height: 0; overflow: hidden; }
.table-card :deep(.n-card-content) { box-sizing: border-box; height: 100%; min-height: 0; }
.address-cell { white-space: nowrap; }
.access-tags { display: inline-flex; flex-wrap: nowrap; gap: 6px; white-space: nowrap; }
tbody tr.manageable { cursor: pointer; }
tbody tr.manageable:hover { background: var(--bg-card-secondary); }
.peer-summary-card { height: 100%; }
.ha-summary { display: flex; flex-wrap: wrap; gap: 16px; margin-bottom: 12px; color: var(--text-muted); }
:global(.manage-modal) { width: min(1400px, calc(100vw - 32px)); max-height: calc(100vh - 32px); overflow: auto; }
:global(.manage-modal table) { width: 100%; table-layout: fixed; }
:global(.manage-modal th), :global(.manage-modal td) { overflow-wrap: anywhere; }
@media (max-width: 768px) { .page-header { align-items: stretch; flex-direction: column; } .page-header .n-button { width: 100%; } :global(.manage-modal) { width: calc(100vw - 16px); max-height: calc(100vh - 16px); } :global(.manage-modal th) { width: auto !important; } :global(.manage-modal table .n-button) { box-sizing: border-box; height: auto; max-width: 100%; padding: 0 4px; white-space: normal; } }
</style>
