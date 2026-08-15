<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h2>Witness 见证仲裁与 GRE 质量监测 (SLA)</h2>
        <span class="sub-title">第三方独立 L3/L4 观测、防脑裂 Quorum 仲裁决策审计与网络质量矩阵</span>
      </div>
      <n-button secondary @click="loadData">刷新监测数据</n-button>
    </div>

    <n-alert :type="quorumAlertType" :title="quorumTitle" class="mb-4">
      <div class="quorum-summary">
        <span>{{ quorum?.decision_reason || '等待兼容 Agent 状态' }}</span>
        <n-tag v-if="quorum?.mode === 'active' || quorum?.policy === 'hub-majority'" size="small" :type="quorumHealthy ? 'success' : 'error'">
          {{ quorum.votes }}/{{ quorum.voters }} · {{ quorum.policy === 'hub-majority' ? 'Hub votes' : `Holder ${quorum.holder || '-'} · ${((quorum.lease_remaining_ms || 0) / 1000).toFixed(1)}s` }}
        </n-tag>
      </div>
      <div v-if="quorum?.members?.length" class="quorum-members">
        <n-tag
          v-for="member in quorum.members"
          :key="member.node_id"
          size="small"
          :type="member.fenced || !member.fresh ? 'error' : member.quorum_available ? 'success' : 'warning'"
        >
          {{ member.member_id || member.node_id }} · peer={{ member.peer_vote ? 'yes' : 'no' }} · manager={{ member.manager_vote ? 'yes' : 'no' }}{{ member.fenced ? ' · FENCED' : '' }}
        </n-tag>
      </div>
    </n-alert>

    <!-- Node SLA Matrix Cards -->
    <n-grid cols="1 s:2 m:3" responsive="screen" :x-gap="16" :y-gap="16" class="mb-4">
      <template v-if="loading && slaList.length === 0">
        <n-grid-item v-for="i in 2" :key="i">
          <n-card class="sla-card">
            <div class="sla-card-header">
              <n-skeleton text style="width: 35%; height: 20px; border-radius: 4px;" />
              <n-skeleton text style="width: 25%; height: 20px; border-radius: 10px;" />
            </div>
            <div class="sla-metrics">
              <div class="metric-item" style="flex: 1;">
                <n-skeleton text style="width: 40px; height: 12px; margin-bottom: 4px;" />
                <n-skeleton text style="width: 70px; height: 24px;" />
              </div>
              <div class="metric-item" style="flex: 1;">
                <n-skeleton text style="width: 40px; height: 12px; margin-bottom: 4px;" />
                <n-skeleton text style="width: 50px; height: 24px;" />
              </div>
            </div>
            <div class="sla-tiers">
              <n-skeleton text style="width: 80px; height: 20px; border-radius: 4px;" />
              <n-skeleton text style="width: 80px; height: 20px; border-radius: 4px;" />
              <n-skeleton text style="width: 60px; height: 20px; border-radius: 4px;" />
            </div>
          </n-card>
        </n-grid-item>
      </template>
      <template v-else>
        <n-grid-item v-for="item in slaList" :key="item.node_id">
          <n-card class="sla-card" :class="item.overall_state || 'healthy'">
            <div class="sla-card-header">
              <div style="display: flex; align-items: center; gap: 6px;">
                <span class="node-title">{{ getNodeDisplayName(item.node_id) }}</span>
                <n-tag v-if="item.firewall_protected" type="info" size="tiny" round>
                  防火墙隔离
                </n-tag>
              </div>
              <n-tag :type="item.overall_state === 'healthy' ? 'success' : item.overall_state === 'degraded' ? 'warning' : 'error'" size="small" round>
                {{ (item.overall_state || 'UNKNOWN').toUpperCase() }}
              </n-tag>
            </div>
            <div class="sla-metrics">
              <div class="metric-item">
                <span class="m-label">
                  平均延迟
                  <span style="font-size: 10px; color: var(--text-muted);">
                    ({{ item.latency_source === 'ws' ? 'WS 遥测' : 'ICMP' }})
                  </span>
                </span>
                <span class="m-val">{{ (item.avg_rtt_ms || 0).toFixed(2) }} ms</span>
              </div>
              <div class="metric-item">
                <span class="m-label">
                  丢包率
                  <span v-if="item.firewall_protected" style="font-size: 10px; color: var(--text-muted);">(心跳在网)</span>
                </span>
                <span class="m-val" :class="{ 'text-error': !item.firewall_protected && (item.loss_rate || 0) > 0 }">
                  {{ ((item.loss_rate || 0) * 100).toFixed(1) }}%
                </span>
              </div>
              <div v-if="item.active_spokes !== undefined && item.active_spokes > 0" class="metric-item">
                <span class="m-label">在线 Spoke</span>
                <span class="m-val" style="color: #3b82f6;">{{ item.active_spokes }}</span>
              </div>
            </div>
            <div class="sla-tiers">
              <!-- L3 -->
              <div class="tier-badge" :class="item.firewall_protected ? 'fw' : item.l3_healthy ? 'ok' : ''">
                {{ item.firewall_protected ? 'L3 入站拦截' : 'L3 物理 NBMA' }}
              </div>
              <!-- L4 -->
              <div class="tier-badge" :class="item.firewall_protected ? 'fw' : item.l4_healthy ? 'ok' : ''">
                {{ item.firewall_protected ? 'L4 端口保护' : 'L4 49002 端口' }}
              </div>
              <!-- Agent: only show when relevant -->
              <div
                v-if="item.agent_healthy || item.firewall_protected"
                class="tier-badge"
                :class="{ ok: item.agent_healthy || item.firewall_protected }"
              >
                Agent 遥测
              </div>
              <div class="tier-badge" :class="{ ok: item.data_healthy }">综合可用</div>
            </div>
          </n-card>
        </n-grid-item>
      </template>
    </n-grid>

    <!-- Arbitration Decisions History -->
    <n-card title="Witness 历史仲裁事件（非当前状态）" class="mb-4">
      <n-alert type="info" :show-icon="false" class="mb-2">
        当前状态以上方 Quorum 状态为准；下表仅记录历史决策与失败原因。
      </n-alert>
      <n-scrollbar style="max-height: 260px;" x-scrollable>
        <n-table :bordered="false" :single-line="true" size="small" style="min-width: 750px;">
          <thead class="sticky-thead">
            <tr>
              <th style="width: 170px;">事件时间</th>
              <th style="width: 80px;">Term</th>
              <th style="width: 180px;">相关 Hub</th>
              <th style="width: 120px;">仲裁决策</th>
              <th style="min-width: 200px;" class="allow-wrap">判定依据与推理</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!arbitrations || arbitrations.length === 0">
              <td colspan="5" class="text-center text-muted">目前主备运行平稳，无异常仲裁与脑裂事件</td>
            </tr>
            <tr v-for="a in arbitrations" :key="a.id">
              <td>{{ a.recorded_at ? new Date(a.recorded_at).toLocaleString() : '' }}</td>
              <td><code>Term {{ a.term }}</code></td>
              <td>{{ arbitrationNodes(a) }}</td>
              <td>
                <n-tag :type="a.decision && a.decision.includes('approve') ? 'success' : a.decision && a.decision.includes('alert') ? 'error' : 'info'" size="small">
                  {{ a.decision || 'N/A' }}
                </n-tag>
              </td>
              <td class="allow-wrap">{{ a.reason }}</td>
            </tr>
          </tbody>
        </n-table>
      </n-scrollbar>
    </n-card>

    <!-- Recent Probes History with server-aggregated timeline -->
    <n-card class="probes-card">
      <template #header>
        <div class="card-header-bar">
          <div class="card-title-group">
            <span class="card-title">探针度量与链路质量时序分析 (Network Activity & SLA)</span>
            <div class="status-indicator">
              <span class="online-tag"><n-icon><PulseOutline /></n-icon>在线监控: {{ healthyNodesCount }} 节点正常</span>
              <span v-if="degradedNodesCount > 0" class="warn-tag">/ {{ degradedNodesCount }} 告警</span>
            </div>
          </div>

          <!-- Controls & Filters (Pill Button Style matching reference) -->
          <div class="chart-controls">
            <!-- Node Selector -->
            <n-select
              v-model:value="selectedNode"
              :options="nodeFilterOptions"
              size="small"
              class="control-node-select"
            />

            <!-- Probe Layer Selector -->
            <n-select
              v-model:value="probeLayer"
              :options="[
                { label: 'L3 物理 Ping', value: 'l3_nbma' },
                { label: 'L4 49002 端口', value: 'l4_port' },
                { label: '全部探针分线', value: 'all' }
              ]"
              size="small"
              class="control-layer-select"
            />

            <!-- Metric Filter (Pill buttons) -->
            <n-radio-group v-model:value="metricType" size="small" class="control-metric-group">
              <n-radio-button value="all">全部</n-radio-button>
              <n-radio-button value="rtt">延迟</n-radio-button>
              <n-radio-button value="loss">丢包率</n-radio-button>
            </n-radio-group>

            <!-- Time Range Segmented Pills (1H / 6H / 12H / 24H) -->
            <n-radio-group v-model:value="timeHours" size="small" class="control-time-group">
              <n-radio-button :value="1">1H</n-radio-button>
              <n-radio-button :value="6">6H</n-radio-button>
              <n-radio-button :value="12">12H</n-radio-button>
              <n-radio-button :value="24">24H</n-radio-button>
            </n-radio-group>
          </div>
        </div>
      </template>

      <!-- Interactive Line Chart View -->
      <div class="chart-wrapper">
        <ProbeLineChart
          :probes="probes"
          :time-hours="timeHours"
          :selected-node="selectedNode"
          :probe-layer="probeLayer"
          :metric-type="metricType"
          :nodes="store.nodes"
        />
      </div>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, computed, onMounted, onUnmounted, watch } from 'vue'
import {
  NGrid,
  NGridItem,
  NCard,
  NTable,
  NTag,
  NButton,
  NSelect,
  NScrollbar,
  NRadioGroup,
  NRadioButton,
  NSkeleton,
  NAlert,
  NIcon,
} from 'naive-ui'
import { PulseOutline } from '@vicons/ionicons5'
import { api } from '../api/client'
import { useAppStore } from '../store'
import ProbeLineChart from '../components/ProbeLineChart.vue'
import type { SLAMatrixItem, ProbeRecord, ArbitrationRecord, WitnessQuorumStatus } from '../types'

const store = useAppStore()

const loading = ref(true)
const slaList = ref<SLAMatrixItem[]>([])
const probes = shallowRef<ProbeRecord[]>([])
const arbitrations = ref<ArbitrationRecord[]>([])
const quorum = ref<WitnessQuorumStatus | null>(null)

const selectedNode = ref<string>('all')
const probeLayer = ref<'l3_nbma' | 'l4_port' | 'all'>('l3_nbma')
const metricType = ref<'all' | 'rtt' | 'loss'>('all')
const timeHours = ref<number>(1)
let probeRequestSequence = 0

const healthyNodesCount = computed(
  () => (slaList.value || []).filter((s) => s && s.overall_state === 'healthy').length
)
const degradedNodesCount = computed(
  () => (slaList.value || []).filter((s) => s && s.overall_state !== 'healthy').length
)
const quorumHealthy = computed(() =>
  (quorum.value?.policy === 'hub-majority' &&
    quorum.value.members.some((m) => m.quorum_available)) ||
  (quorum.value?.policy !== 'hub-majority' && quorum.value?.mode === 'legacy') ||
  (quorum.value?.mode === 'active' && quorum.value.members.some((m) => m.quorum_available)))
const quorumAlertType = computed(() => quorum.value?.policy !== 'hub-majority' && quorum.value?.mode === 'legacy'
  ? 'info'
  : quorumHealthy.value ? 'success' : quorum.value?.mode === 'active' ? 'error' : 'warning')
const quorumTitle = computed(() => {
	if (quorum.value?.policy === 'hub-majority') {
		return quorumHealthy.value
			? `Hub Majority ${quorum.value.votes}/${quorum.value.voters} 正常`
			: `Hub Majority ${quorum.value.votes}/${quorum.value.required} 不足`
	}
  if (!quorum.value || quorum.value.mode === 'legacy') return 'Legacy availability-first'
  if (quorum.value.mode === 'preparing') return 'Witness preparing'
  if (quorum.value.mode === 'disabling') return 'Witness disabling'
  return quorumHealthy.value ? 'Witness Quorum 2/3 正常' : 'Witness 无多数派'
})

const getNodeDisplayName = (nodeId: string) => {
  const n = store.nodes.find((item) => item.id === nodeId)
  return n?.name || nodeId
}

const arbitrationNodes = (record: ArbitrationRecord) => {
  if (record.involved_node_ids?.length) {
    return record.involved_node_ids.map(getNodeDisplayName).join('、')
  }
  if (record.decision === 'witness_disabled' && record.reason.includes('three or more')) {
    return '全部 online Hub'
  }
  return [...new Set([record.primary_node_id, record.backup_node_id].filter(Boolean))]
    .map(getNodeDisplayName)
    .join('、') || '-'
}

const nodeFilterOptions = computed(() => {
  const options = [{ label: '全部 Hub 节点', value: 'all' }]
  if (Array.isArray(slaList.value)) {
    slaList.value.forEach((s) => {
      if (s && s.node_id) {
        options.push({
          label: getNodeDisplayName(s.node_id),
          value: s.node_id,
        })
      }
    })
  }
  return options
})

const loadProbes = async () => {
  const sequence = ++probeRequestSequence
  try {
    const data = await api.getRecentProbes(selectedNode.value, timeHours.value, probeLayer.value, 200)
    if (sequence === probeRequestSequence) {
      probes.value = Array.isArray(data) ? data : []
    }
  } catch (e) {
    if (sequence === probeRequestSequence) {
      probes.value = []
    }
    console.error('Failed to load witness probes', e)
  }
}

const loadData = async () => {
  try {
    const [sla, arb] = await Promise.all([
      api.getSLAMatrix().catch(() => []),
      api.getArbitrations(20).catch(() => []),
    ])

    slaList.value = Array.isArray(sla) ? sla : []
    arbitrations.value = Array.isArray(arb) ? arb : []
  } catch (e) {
    console.error('Failed to load witness data', e)
  } finally {
    loading.value = false
  }
}

const loadQuorum = async () => {
  quorum.value = await api.getWitnessQuorum().catch(() => null)
}

watch([selectedNode, probeLayer, timeHours], loadProbes)

onMounted(() => {
  loadData()
  loadQuorum()
  quorumTimer = window.setInterval(loadQuorum, 1000)
  loadProbes()
  if (store.nodes.length === 0) {
    store.fetchNodes()
  }
})

let quorumTimer: number | null = null
onUnmounted(() => {
  if (quorumTimer !== null) window.clearInterval(quorumTimer)
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
  font-size: 20px;
  color: var(--text-title);
}

.sub-title {
  font-size: 13px;
  color: var(--text-muted);
}

.quorum-summary,
.quorum-members {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.quorum-summary {
  justify-content: space-between;
}

.quorum-members {
  margin-top: 10px;
}

.sla-card {
  height: 100%;
  min-height: 146px;
  border-radius: 8px;
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  transition: all 0.2s ease;
}

.sla-card :deep(.n-card__content) {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  height: 100%;
  box-sizing: border-box;
}

.sla-card.healthy {
  border-top: 3px solid #10b981;
}

.sla-card.degraded {
  border-top: 3px solid #f59e0b;
}

.sla-card.critical,
.sla-card.error,
.sla-card.down {
  border-top: 3px solid #ef4444;
}

.sla-card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
  min-height: 24px;
}

.node-title {
  font-weight: 700;
  font-size: 15px;
  color: var(--text-title);
}

.sla-metrics {
  display: flex;
  gap: 24px;
  margin-bottom: 12px;
  min-height: 42px;
}

.metric-item {
  display: flex;
  flex-direction: column;
}

.m-label {
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.2;
}

.m-val {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-title);
  line-height: 1.3;
}

.sla-tiers {
  display: flex;
  gap: 6px;
  min-height: 22px;
  align-items: center;
}

.tier-badge {
  font-size: 10px;
  padding: 3px 6px;
  border-radius: 4px;
  background: var(--bg-card-secondary);
  color: var(--text-muted);
}

.tier-badge.ok {
  background: rgba(16, 185, 129, 0.12);
  color: #10b981;
  font-weight: 600;
}

.tier-badge.fw {
  background: rgba(59, 130, 246, 0.12);
  color: #3b82f6;
  font-weight: 500;
}

.text-error {
  color: #ef4444 !important;
}

.sticky-thead th {
  position: sticky;
  top: 0;
  background: var(--bg-card) !important;
  z-index: 2;
  box-shadow: 0 1px 0 var(--border-color);
}

.probes-card {
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 8px;
}

.card-header-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

.card-title-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.card-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--text-title);
}

.status-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
}

.online-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #10b981;
  font-weight: 600;
}

.warn-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: #ef4444;
  font-weight: 600;
}

.chart-controls {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.control-node-select {
  width: 160px;
}

.control-layer-select {
  width: 150px;
}

.chart-wrapper {
  padding-top: 4px;
}

.mb-4 {
  margin-bottom: 16px;
}

@media (max-width: 768px) {
  .page-container {
    padding: 12px;
  }
  .page-header {
    flex-direction: column;
    align-items: stretch !important;
    gap: 12px;
  }
  .page-header h2 {
    font-size: 18px;
  }
  .page-header .n-button {
    width: 100% !important;
    justify-content: center !important;
  }
  .card-header-bar {
    flex-direction: column;
    align-items: flex-start;
    gap: 12px;
  }
  .card-title-group {
    width: 100%;
  }
  .chart-controls {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
    width: 100%;
  }
  .control-node-select,
  .control-layer-select {
    width: 100% !important;
  }
  .control-metric-group,
  .control-time-group {
    grid-column: span 2;
    display: flex;
    width: 100%;
  }
  .control-metric-group :deep(.n-radio-button),
  .control-time-group :deep(.n-radio-button) {
    flex: 1;
    text-align: center;
    display: flex;
    justify-content: center;
    align-items: center;
  }
  .sla-tiers {
    flex-wrap: wrap;
  }
}
</style>
