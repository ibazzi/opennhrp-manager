<template>
  <div class="page-container">
    <div class="page-header">
      <div class="header-left">
        <h2>全局运行概览</h2>
        <span class="sub-title">OpenNHRP 托管多 Hub HA 集群与全网数据面健康中枢</span>
      </div>
      <div class="header-right">
        <n-space align="center">
          <template v-if="loading && !cluster">
            <n-skeleton text style="width: 105px; height: 28px; border-radius: 14px;" />
            <n-skeleton text style="width: 115px; height: 28px; border-radius: 14px;" />
          </template>
          <template v-else>
            <n-tag :type="clusterServiceAvailable ? 'success' : 'error'" round>
              <n-icon><component :is="clusterServiceAvailable ? CheckmarkCircleOutline : CloseCircleOutline" /></n-icon>
              {{ clusterServiceAvailable ? '集群服务可用' : '集群服务不可用' }}
            </n-tag>
            <n-tag :type="cluster?.network_health_status === 'healthy' ? 'success' : cluster?.network_health_status === 'disabled' ? 'info' : 'warning'" round>
              <n-icon><PulseOutline /></n-icon>
              网关探测: {{ networkHealthText }}
            </n-tag>
          </template>
          <n-button size="small" secondary @click="refreshData">刷新数据</n-button>
        </n-space>
      </div>
    </div>

    <!-- Stat Cards -->
    <n-grid cols="1 s:2 m:4" responsive="screen" :x-gap="16" :y-gap="16" class="stat-grid">
      <n-grid-item>
        <n-card size="small" class="stat-card">
          <div class="stat-label">当前节点 HA 角色</div>
          <div class="stat-value highlight-role" :class="cluster?.local_role">
            <n-skeleton v-if="loading && !cluster" text style="width: 70%; height: 18px; border-radius: 4px;" />
            <span v-else>{{ formatRole(cluster?.local_role) }}</span>
          </div>
          <div class="stat-meta">
            <n-skeleton v-if="loading && !cluster" text style="width: 85%; height: 12px; border-radius: 3px;" />
            <span v-else>Member: {{ cluster?.member || store.activeNodeId }} | Term: {{ cluster?.term || 0 }}{{ cluster?.stale ? ' | 缓存' : '' }}</span>
          </div>
        </n-card>
      </n-grid-item>

      <n-grid-item>
        <n-card size="small" class="stat-card">
          <div class="stat-label">当前集群 Leader</div>
          <div class="stat-value text-emerald">
            <n-skeleton v-if="loading && !cluster" text style="width: 65%; height: 18px; border-radius: 4px;" />
            <span v-else class="icon-value"><n-icon><TrophyOutline /></n-icon>{{ cluster?.leader || '-' }}</span>
          </div>
          <div class="stat-meta">
            <n-skeleton v-if="loading && !cluster" text style="width: 90%; height: 12px; border-radius: 3px;" />
            <span v-else>Commit Index: {{ cluster?.commit_index ?? 0 }} | Rev: {{ cluster?.manifest_revision ?? 0 }}</span>
          </div>
        </n-card>
      </n-grid-item>

      <n-grid-item>
        <n-card size="small" class="stat-card">
          <div class="stat-label">活跃 Spoke 客户端</div>
          <div class="stat-value text-purple">
            <n-skeleton v-if="loading && !cluster" text style="width: 50%; height: 18px; border-radius: 4px;" />
            <span v-else>{{ spokes.length }} 台</span>
          </div>
          <div class="stat-meta">
            <n-skeleton v-if="loading && !cluster" text style="width: 80%; height: 12px; border-radius: 3px;" />
            <span v-else>动态注册: {{ directCount }} | 静态/影子: {{ shadowCount }}{{ spokesStale ? ' | 缓存' : '' }}</span>
          </div>
        </n-card>
      </n-grid-item>

      <n-grid-item>
        <n-card size="small" class="stat-card">
          <div class="stat-label">Witness 见证状态</div>
          <div class="stat-value text-amber">
            <n-skeleton v-if="loading && !cluster" text style="width: 65%; height: 18px; border-radius: 4px;" />
            <span v-else>{{ witnessLabel }}</span>
          </div>
          <div class="stat-meta">
            <n-skeleton v-if="loading && slaMatrix.length === 0" text style="width: 70%; height: 12px; border-radius: 3px;" />
            <span v-else>{{ witnessMeta }}</span>
          </div>
        </n-card>
      </n-grid-item>
    </n-grid>

    <!-- Topology & Side Detail Cards Section -->
    <div class="topology-detail-layout mt-4">
      <!-- Left: Topology Graph -->
      <div class="topology-left-col">
        <div class="section-title">动态集群与网络拓扑</div>
        <TopologyGraph
          :active-node-id="store.activeNodeId"
          :cluster-status="cluster"
          :witness-quorum="witnessQuorum"
          :spokes="spokes"
          :sla-matrix="slaMatrix"
          @select-node="handleSelectNode"
        />
      </div>

      <!-- Right: Side Detail Cards (50% / 50% vertical split) -->
      <div class="topology-right-col">
        <!-- Top Half: HA Health Targets -->
        <div class="side-panel-block">
          <div class="section-title">HA 健康探测目标 (ha-health-target)</div>
          <div class="side-detail-card">
            <n-scrollbar x-scrollable :size="6" class="side-card-scroll" style="height: 100%; max-height: 100%;">
              <n-table size="small" :bordered="false" :single-line="true" class="side-table" style="min-width: 320px;">
                <thead>
                  <tr>
                    <th style="width: 140px;">目标单播 IP</th>
                    <th style="width: 80px;">状态</th>
                    <th style="width: 90px;">探测间隔</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-if="loading && (!cluster?.health_targets || cluster?.health_targets?.length === 0)">
                    <td colspan="3" class="text-center text-muted">
                      <n-skeleton text :repeat="2" style="margin: 6px 0;" />
                    </td>
                  </tr>
                  <tr v-else-if="!cluster?.health_targets || cluster?.health_targets?.length === 0">
                    <td colspan="3" class="text-center text-muted">未配置 HA 健康探测目标</td>
                  </tr>
                  <tr v-for="target in cluster?.health_targets" :key="target.target_ip">
                    <td><code>{{ target.target_ip }}</code></td>
                    <td>
                      <n-badge :type="target.last_success ? 'success' : 'error'" :value="target.last_success ? 'OK' : 'FAIL'" />
                    </td>
                    <td>{{ target.interval_sec }}s</td>
                  </tr>
                </tbody>
              </n-table>
            </n-scrollbar>
          </div>
        </div>

        <!-- Bottom Half: Quick Spokes -->
        <div class="side-panel-block">
          <div class="section-title">最近在线 Spoke 列表</div>
          <div class="side-detail-card">
            <n-scrollbar x-scrollable :size="6" class="side-card-scroll" style="height: 100%; max-height: 100%;">
              <n-table size="small" :bordered="false" :single-line="true" class="side-table" style="min-width: 440px;">
                <thead>
                  <tr>
                    <th style="width: 130px;">Protocol IP</th>
                    <th style="width: 140px;">NBMA 外网物理 IP</th>
                    <th style="width: 70px;">类型</th>
                    <th style="width: 80px;">租约剩余</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-if="loading && spokes.length === 0">
                    <td colspan="4" class="text-center text-muted">
                      <n-skeleton text :repeat="2" style="margin: 6px 0;" />
                    </td>
                  </tr>
                  <tr v-else-if="spokes.length === 0">
                    <td colspan="4" class="text-center text-muted">暂无活跃注册 Spoke</td>
                  </tr>
                  <tr v-for="spoke in spokes" :key="spoke.protocol_address">
                    <td><strong>{{ spoke.protocol_address }}</strong></td>
                    <td><code>{{ spoke.nbma_address }}</code></td>
                    <td>
                      <n-tag size="tiny" :type="spoke.type === 'direct' ? 'success' : 'info'">
                        {{ spoke.type }}
                      </n-tag>
                    </td>
                    <td>{{ spoke.expires_in_sec }}s</td>
                  </tr>
                </tbody>
              </n-table>
            </n-scrollbar>
          </div>
        </div>
      </div>
    </div>

    <!-- Bottom Log Terminal -->
    <div class="log-section">
      <TerminalLog
        title="实时事件与协调器控制日志 (Real-time Live Stream)"
        style="height: 320px;"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch } from 'vue'
import {
  NGrid,
  NGridItem,
  NCard,
  NTag,
  NBadge,
  NButton,
  NTable,
  NSpace,
  NScrollbar,
  NSkeleton,
  NIcon,
  useMessage,
} from 'naive-ui'
import { CheckmarkCircleOutline, CloseCircleOutline, PulseOutline, TrophyOutline } from '@vicons/ionicons5'
import TopologyGraph from '../components/TopologyGraph.vue'
import TerminalLog from '../components/TerminalLog.vue'
import { api } from '../api/client'
import { useAppStore } from '../store'
import { sortSpokesByIP } from '../utils/ip'
import { formatWitnessQuorumStatus } from '../utils/topologyStatus'
import type { ClusterStatus, SpokeInfo, SLAMatrixItem, TopologySnapshot, WitnessQuorumStatus } from '../types'

const store = useAppStore()
const message = useMessage()

const loading = ref(true)
const cluster = ref<ClusterStatus | null>(null)
const witnessQuorum = ref<WitnessQuorumStatus | null>(null)
const spokes = ref<SpokeInfo[]>([])
const slaMatrix = ref<SLAMatrixItem[]>([])
const clusterServiceAvailable = computed(() => Boolean(
  cluster.value?.service_available &&
  !cluster.value?.isolated &&
  (cluster.value?.witness?.quorum_available ?? true),
))

const handleSelectNode = (node: any) => {
  if (node && node.id && (node.type === 'leader' || node.type === 'standby' || node.type === 'hub')) {
    const matched = store.nodes.find(
      (n) => n.id === node.id || n.name === node.id || n.name === node.title || (node.subtitle1 && (n.host === node.subtitle1 || n.advertised_ip === node.subtitle1))
    )
    const targetId = matched?.id || node.id
    if (store.activeNodeId !== targetId) {
      store.activeNodeId = targetId
      message.success(`已切换管理视图至 Hub 节点: ${matched?.name || node.title || targetId}`)
    }
  }
}

const directCount = computed(() => spokes.value.filter((s) => s.type === 'dynamic' || s.type === 'direct').length)
const shadowCount = computed(() => spokes.value.filter((s) => s.type === 'shadow' || s.type === 'static').length)
const spokesStale = computed(() => spokes.value.some((s) => s.stale))

const witnessLabel = computed(() => {
  return formatWitnessQuorumStatus(witnessQuorum.value)
})

const witnessMeta = computed(() => {
  if (!witnessQuorum.value) return 'Manager 全局仲裁状态不可用'
  return witnessQuorum.value.decision_reason ||
    `Leader: ${witnessQuorum.value.leader || '-'} | Term: ${witnessQuorum.value.term}`
})

const networkHealthText = computed(() => {
  switch (cluster.value?.network_health_status) {
    case 'healthy': return '正常'
    case 'unhealthy': return '异常'
    case 'disabled': return '未启用'
    default: return '未知'
  }
})

const formatRole = (role?: string) => {
  switch (role) {
    case 'leader': return 'Leader 活跃'
    case 'standby': return 'Standby 备用'
    case 'learner': return 'Learner 同步中'
    case 'witness': return 'Witness 见证仲裁'
    case 'isolated': return 'Isolated 隔离态'
    default: return '未初始化 / 单机'
  }
}

const refreshData = async () => {
  try {
    await store.fetchNodes()
    const targetNode = store.activeNodeId
    const [statusData, spokesData, slaData, quorumData] = await Promise.all([
      api.getClusterStatus(targetNode),
      api.listSpokes(targetNode),
      api.getSLAMatrix(),
      api.getWitnessQuorum().catch(() => null),
    ])
    cluster.value = statusData
    spokes.value = sortSpokesByIP(spokesData)
    slaMatrix.value = slaData
    witnessQuorum.value = quorumData
  } catch (e) {
    console.error('Refresh dashboard error', e)
  } finally {
    loading.value = false
  }
}

watch(
  () => store.activeNodeId,
  () => {
    connectTopologyWS()
  }
)

let topologyWS: WebSocket | null = null
let reconnectTimer: number | null = null

const closeTopologyWS = () => {
  if (reconnectTimer) window.clearTimeout(reconnectTimer)
  reconnectTimer = null
  if (topologyWS) {
    topologyWS.onclose = null
    topologyWS.close()
    topologyWS = null
  }
}

const connectTopologyWS = () => {
  closeTopologyWS()
  const token = localStorage.getItem('opennhrp_token')
  if (!token || !store.activeNodeId) return
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  topologyWS = new WebSocket(
    `${protocol}//${window.location.host}/api/topology/ws?node_id=${encodeURIComponent(store.activeNodeId)}&token=${encodeURIComponent(token)}`
  )
  topologyWS.onmessage = (event) => {
    const snapshot = JSON.parse(event.data) as TopologySnapshot
    if (snapshot.node_id && snapshot.node_id !== store.activeNodeId) return
    store.nodes = snapshot.nodes || []
    if (snapshot.cluster) cluster.value = snapshot.cluster
    spokes.value = sortSpokesByIP(snapshot.spokes || [])
    slaMatrix.value = snapshot.sla_matrix || []
    witnessQuorum.value = snapshot.witness_quorum || null
    loading.value = false
  }
  topologyWS.onclose = () => {
    if (localStorage.getItem('opennhrp_token')) {
      reconnectTimer = window.setTimeout(connectTopologyWS, 3000)
    }
  }
}

onMounted(async () => {
  await refreshData()
  connectTopologyWS()
})

onUnmounted(() => {
  closeTopologyWS()
})
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

.stat-grid {
  margin-bottom: 16px;
}

.stat-card {
  height: 100%;
  min-height: 76px;
  max-height: 76px;
  background: var(--bg-card) !important;
  border: 1px solid var(--border-color) !important;
  border-radius: 8px;
  box-shadow: var(--card-shadow);
  transition: background-color 0.2s ease, border-color 0.2s ease;
  box-sizing: border-box;
  overflow: hidden;
}

.stat-card :deep(.n-card__content) {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 2px;
  height: 100%;
  padding: 8px 14px !important;
  box-sizing: border-box;
}

.stat-label {
  font-size: 11px;
  line-height: 14px;
  height: 14px;
  color: var(--text-muted);
  white-space: nowrap;
}

.stat-value {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-title);
  height: 22px;
  min-height: 22px;
  line-height: 22px;
  display: flex;
  align-items: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin: 0;
}

.icon-value {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.stat-meta {
  font-size: 10.5px;
  line-height: 14px;
  height: 14px;
  min-height: 14px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: flex;
  align-items: center;
}

.highlight-role.leader {
  color: #10b981;
}

.highlight-role.standby {
  color: #3b82f6;
}

.highlight-role.witness {
  color: #f59e0b;
}

.highlight-role.isolated {
  color: #ef4444;
}

.text-emerald {
  color: #10b981;
}

.text-purple {
  color: #a855f7;
}

.text-amber {
  color: #f59e0b;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-title);
  margin-top: 0;
  margin-bottom: 8px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.topology-detail-layout {
  display: flex;
  gap: 16px;
  align-items: stretch;
  min-height: 480px;
}

.topology-left-col {
  flex: 1 1 65%;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.topology-left-col > .topology-container {
  height: 100%;
  flex: 1;
}

.topology-right-col {
  flex: 0 0 35%;
  min-width: 320px;
  max-width: 35%;
  display: flex;
  flex-direction: column;
  gap: 14px;
  height: auto;
  overflow: hidden;
}

.side-panel-block {
  flex: 1 1 0;
  min-height: 0;
  height: 50%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.side-detail-card {
  flex: 1 1 0;
  min-height: 0;
  height: 100%;
  max-height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  box-shadow: var(--card-shadow);
  transition: background-color 0.2s ease, border-color 0.2s ease;
  overflow: hidden;
  box-sizing: border-box;
}

.side-card-scroll {
  flex: 1 1 0;
  min-height: 0;
  height: 100%;
  max-height: 100%;
  width: 100%;
  box-sizing: border-box;
}

.side-card-scroll :deep(.n-scrollbar-container) {
  min-height: 0;
  height: 100%;
  max-height: 100%;
}

.side-card-scroll :deep(.n-scrollbar-content) {
  min-height: 100%;
}

.side-card-scroll :deep(.n-scrollbar-rail) {
  opacity: 0.8 !important;
  transition: opacity 0.2s ease;
}

.side-card-scroll :deep(.n-scrollbar-rail:hover) {
  opacity: 1 !important;
}

.side-card-scroll :deep(.n-scrollbar-rail__scrollbar) {
  background-color: rgba(140, 140, 150, 0.5) !important;
  border-radius: 3px !important;
}

.side-table {
  width: 100%;
}

.side-table thead,
.side-table thead tr {
  position: sticky !important;
  top: 0 !important;
  z-index: 10 !important;
}

.side-table thead th {
  position: sticky !important;
  top: 0 !important;
  background: var(--bg-card) !important;
  z-index: 10 !important;
  font-weight: 600;
  font-size: 11.5px;
  box-shadow: 0 1px 0 var(--border-color) !important;
  padding: 6px 8px !important;
}

.side-table tbody td {
  padding: 6px 8px !important;
  font-size: 12px;
}

.log-section {
  margin-top: 24px;
  margin-bottom: 20px;
}

.text-center {
  text-align: center;
}

.text-muted {
  color: var(--text-muted);
}

@media (max-width: 1100px) {
  .topology-detail-layout {
    flex-direction: column;
    gap: 16px;
    min-height: 0;
  }
  .topology-left-col {
    width: 100%;
    flex: none;
  }
  .topology-right-col {
    width: 100%;
    max-width: 100%;
    min-width: 0;
    flex: none;
    gap: 16px;
  }
  .side-panel-block {
    flex: none;
    width: 100%;
    height: auto;
  }
  .side-detail-card {
    flex: none;
    height: 220px;
    max-height: 220px;
    width: 100%;
  }
}

@media (max-width: 768px) {
  .page-header {
    flex-direction: column;
    align-items: stretch !important;
    gap: 12px;
    margin-bottom: 12px;
  }
  .header-right {
    width: 100%;
  }
  .header-right .n-space {
    width: 100%;
    flex-direction: column !important;
    align-items: stretch !important;
    gap: 8px !important;
  }
  .header-right .n-space > * {
    width: 100% !important;
  }
  .header-right .n-button {
    width: 100% !important;
    justify-content: center !important;
  }
  .header-right .n-tag {
    width: 100% !important;
    justify-content: center !important;
  }
  .stat-value {
    font-size: 17px;
  }
}
</style>
