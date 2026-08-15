<template>
  <div class="page-container">
    <div class="page-header">
      <div>
        <h2>Hub HA 集群管理</h2>
        <span class="sub-title">全域多 Hub 高可用集群共识、动态入网邀请、分布式数据复制与协同治理</span>
      </div>
      <n-space>
        <n-button
          type="primary"
          secondary
          :disabled="!store.isAdmin"
          :title="!store.isAdmin ? '只读用户无权操作' : ''"
          @click="showInviteModal = true"
        >
          + 创建入网邀请 (Invite)
        </n-button>
        <n-button
          type="info"
          secondary
          :disabled="!store.isAdmin"
          :title="!store.isAdmin ? '只读用户无权操作' : ''"
          @click="showJoinModal = true"
        >
          节点加入集群 (Join)
        </n-button>
        <n-button
          type="warning"
          secondary
          :disabled="!store.isAdmin"
          :title="!store.isAdmin ? '只读用户无权操作' : ''"
          @click="handleFailback"
        >
          请求回切到 Primary
        </n-button>
      </n-space>
    </div>

    <!-- Cluster Overview Cards (4 Balanced Columns) -->
    <n-grid cols="1 s:2 m:4" responsive="screen" :x-gap="16" :y-gap="16" class="mb-4">
      <!-- Card 1: Cluster & Consensus -->
      <n-grid-item>
        <n-card size="small" class="cluster-summary-card">
          <div class="card-header-label">集群共识与 Leader</div>
          <div class="card-large-text text-emerald">
            <n-skeleton v-if="loading && !cluster" text style="width: 60%; height: 20px; border-radius: 4px;" />
            <span v-else class="icon-value"><n-icon><TrophyOutline /></n-icon>{{ cluster?.leader || '-' }}</span>
          </div>
          <div class="card-meta-list">
            <template v-if="loading && !cluster">
              <div class="sub-info"><n-skeleton text style="width: 80%; height: 13px;" /></div>
              <div class="sub-info"><n-skeleton text style="width: 90%; height: 13px;" /></div>
            </template>
            <template v-else>
              <div class="sub-info">ID: <code>{{ shortValue(cluster?.cluster_id) }}</code> | Primary: {{ cluster?.primary || '-' }}</div>
              <div class="sub-info">活跃: {{ haOnlineCount }} / 共 {{ members.length }} 节点 | Term {{ cluster?.term || 0 }} · Commit {{ cluster?.commit_index ?? 0 }}</div>
            </template>
          </div>
        </n-card>
      </n-grid-item>

      <!-- Card 2: Cluster Service & Health -->
      <n-grid-item>
        <n-card size="small" class="cluster-summary-card">
          <div class="card-header-label">集群服务与网络健康</div>
          <div class="card-large-text">
            <n-skeleton v-if="loading && !cluster" text style="width: 50%; height: 20px; border-radius: 4px;" />
            <span v-else>{{ clusterServiceAvailable ? '全域就绪' : '服务降级' }}</span>
          </div>
          <div class="card-meta-list">
            <template v-if="loading && !cluster">
              <div class="tag-row">
                <n-skeleton style="width: 65px; height: 18px; border-radius: 4px;" />
                <n-skeleton style="width: 75px; height: 18px; border-radius: 4px;" />
              </div>
              <div class="sub-info"><n-skeleton text style="width: 65%; height: 13px;" /></div>
            </template>
            <template v-else>
              <div class="tag-row">
                <n-tag size="tiny" :type="clusterServiceAvailable ? 'success' : 'error'">
                  服务: {{ clusterServiceAvailable ? '可用' : '不可用' }}
                </n-tag>
                <n-tag size="tiny" :type="cluster?.isolated ? 'error' : 'success'">
                  {{ cluster?.isolated ? '防脑裂隔离中' : '共识正常' }}
                </n-tag>
                <n-tag size="tiny" :type="networkHealthType">{{ networkHealthText }}</n-tag>
              </div>
              <div class="sub-info">
                探测周期: {{ cluster?.health_interval_seconds ?? 0 }}s
                <template v-if="cluster?.health_targets?.length">
                  |
                  <span v-for="target in cluster.health_targets" :key="target.target_ip" class="mr-1 target-item">
                    {{ target.target_ip }}
                    <n-icon :color="target.last_success ? '#18a058' : '#d03050'">
                      <component :is="target.last_success ? CheckmarkCircleOutline : CloseCircleOutline" />
                    </n-icon>
                  </span>
                </template>
              </div>
            </template>
          </div>
        </n-card>
      </n-grid-item>

      <!-- Card 3: Replication Status -->
      <n-grid-item>
        <n-card size="small" class="cluster-summary-card">
          <div class="card-header-label">数据复制与同步</div>
          <div class="card-large-text">
            <n-skeleton v-if="loading && !replication" text style="width: 55%; height: 20px; border-radius: 4px;" />
            <span v-else>Index {{ replication?.local_index ?? '-' }}</span>
          </div>
          <div class="card-meta-list">
            <template v-if="loading && !replication">
              <div class="sub-info"><n-skeleton text style="width: 85%; height: 13px;" /></div>
              <div class="sub-info"><n-skeleton text style="width: 70%; height: 13px;" /></div>
            </template>
            <template v-else>
              <div class="sub-info">快照: 收 {{ replication?.snapshots_received ?? 0 }} / 发 {{ replication?.snapshots_sent ?? 0 }} | 增量: 收 {{ replication?.deltas_received ?? 0 }} / 发 {{ replication?.deltas_sent ?? 0 }}</div>
              <div class="sub-info">Digest: {{ shortValue(replication?.digest) }} | 重同步: {{ replication?.resync_requests ?? 0 }}</div>
            </template>
          </div>
        </n-card>
      </n-grid-item>

      <!-- Card 4: PSK Key Status -->
      <n-grid-item>
        <n-card size="small" class="cluster-summary-card">
          <div class="card-header-label">PSK 密钥状态</div>
          <div class="card-large-text text-amber">
            <n-skeleton v-if="loading && !keyStatus" text style="width: 60%; height: 20px; border-radius: 4px;" />
            <span v-else>{{ keyStatus?.current_key_id ? shortValue(keyStatus.current_key_id) : '-' }}</span>
          </div>
          <div class="card-meta-list">
            <template v-if="loading && !keyStatus">
              <div class="sub-info"><n-skeleton text style="width: 60%; height: 13px;" /></div>
              <div class="sub-info"><n-skeleton text style="width: 80%; height: 13px;" /></div>
            </template>
            <template v-else>
              <div class="sub-info">Next: {{ shortValue(keyStatus?.next_key_id) || '未就绪' }}</div>
              <div class="sub-info action-row">
                <n-button text size="tiny" type="primary" :disabled="!store.isAdmin" @click="handleRotateKey('prepare')">准备新密钥</n-button>
                <span class="divider">|</span>
                <n-button text size="tiny" type="warning" :disabled="!store.isAdmin" @click="handleRotateKey('commit')">提交轮转</n-button>
                <span class="divider">|</span>
                <a :href="`/api/cluster/key/export-spoke?node_id=${clusterTargetNodeId}`" target="_blank" class="download-link">下载 Keyring</a>
              </div>
            </template>
          </div>
        </n-card>
      </n-grid-item>
    </n-grid>

    <!-- Members Table -->
    <n-card title="集群 Hub 成员列表 (Members)" class="mb-4">
      <n-scrollbar x-scrollable>
        <n-table :bordered="false" :single-line="true" style="min-width: 1280px;">
          <thead>
            <tr>
              <th style="width: 180px;">Member ID</th>
              <th style="width: 120px;">HA 实时会话</th>
              <th style="width: 150px;">Manager Agent</th>
              <th style="width: 100px;">成员资格</th>
              <th style="width: 90px;">优先级</th>
              <th style="width: 170px;">宣告外网地址 (Advertised)</th>
              <th style="width: 150px;">学习源地址 (Observed)</th>
              <th style="width: 180px;">复制进度 (Match / Lag)</th>
              <th style="width: 180px;">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading && members.length === 0">
              <td colspan="9" class="text-center text-muted">
                <n-skeleton text :repeat="3" style="margin: 12px 0;" />
              </td>
            </tr>
            <tr v-else-if="members.length === 0">
              <td colspan="9" class="text-center text-muted">暂无可用的集群成员数据</td>
            </tr>
            <tr v-for="m in members" :key="m.member_id">
              <td>
                <strong>{{ m.member_id }}</strong>
                <n-tag v-if="m.member_id === cluster?.leader" size="tiny" type="success" class="ml-2">LEADER</n-tag>
                <n-tag v-else-if="m.member_id === cluster?.primary" size="tiny" type="info" class="ml-2">PRIMARY</n-tag>
              </td>
              <td>
                <n-tag :type="isMemberHAOnline(m) ? 'success' : m.connected ? 'warning' : 'error'" size="small">
                  {{ memberHAStatus(m) }}
                </n-tag>
              </td>
              <td>
                <n-tag v-if="managerNode(m)" :type="managerNode(m)?.status === 'online' ? 'success' : 'error'" size="small">
                  {{ managerNode(m)?.status === 'online' ? '在线' : '离线' }}
                </n-tag>
                <n-tag v-else size="small">未接入</n-tag>
                <div v-if="managerNode(m)" class="sub-info">{{ formatLastSeen(managerNode(m)?.last_seen) }}</div>
              </td>
              <td>
                <n-tag :type="m.state === 'active' ? 'success' : m.state === 'learner' ? 'warning' : 'error'" size="small">{{ m.state }}</n-tag>
              </td>
              <td>{{ m.priority }}</td>
              <td>
                <code v-for="ip in m.advertised_addresses" :key="ip" class="mr-1">{{ ip }}</code>
                <span v-if="!m.advertised_addresses?.length" class="text-muted">-</span>
              </td>
              <td><code>{{ m.observed_address || '-' }}</code></td>
              <td>
                <span v-if="m.member_id === cluster?.leader">Leader Idx: {{ m.match_index ?? 0 }}</span>
                <span v-else>Idx: {{ m.match_index ?? 0 }} (Lag: {{ m.lag ?? 0 }})</span>
                <div class="sub-info">Digest: {{ shortValue(replicationDigest(m.member_id)) }}</div>
              </td>
              <td>
                <n-space>
                  <n-button size="tiny" secondary :disabled="!store.isAdmin" @click="openEditPriority(m)">修改优先级</n-button>
                  <n-button size="tiny" :type="m.state === 'disabled' ? 'success' : 'error'" secondary :disabled="!store.isAdmin" @click="handleSetMemberDisabled(m.member_id, m.state !== 'disabled')">{{ m.state === 'disabled' ? '启用' : '禁用' }}</n-button>
                </n-space>
              </td>
            </tr>
          </tbody>
        </n-table>
      </n-scrollbar>
    </n-card>

    <!-- Invite Tokens Table -->
    <n-card title="入网邀请令牌记录 (Invite Tokens)">
      <n-scrollbar x-scrollable>
        <n-table :bordered="false" :single-line="true" style="min-width: 780px;">
          <thead>
            <tr>
              <th style="width: 120px;">ID 前缀</th>
              <th style="width: 150px;">目标 Member ID</th>
              <th style="width: 100px;">预设优先级</th>
              <th style="width: 90px;">状态</th>
              <th style="width: 170px;">过期时间</th>
              <th style="width: 150px;">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="invites.length === 0">
              <td colspan="6" class="text-center text-muted">暂无有效 Invite 记录</td>
            </tr>
            <tr v-for="inv in invites" :key="inv.id_prefix">
              <td><code>{{ inv.id_prefix }}...</code></td>
              <td><strong>{{ inv.member_id }}</strong></td>
              <td>{{ inv.priority }}</td>
              <td>
                <n-tag
                  size="small"
                  :type="inv.state === 'unused' ? 'info' : inv.state === 'claimed' ? 'success' : inv.state === 'revoked' ? 'error' : 'warning'"
                >
                  {{ inv.state === 'unused' ? '未使用' : inv.state === 'claimed' ? '已声明' : inv.state === 'revoked' ? '已撤销' : '已过期' }}
                </n-tag>
              </td>
              <td>{{ new Date(inv.expires_at).toLocaleString() }}</td>
              <td>
                <n-space size="small">
                  <n-button
                    v-if="inv.state === 'unused'"
                    size="tiny"
                    type="warning"
                    secondary
                    :disabled="!store.isAdmin"
                    @click="handleRevokeInvite(inv.id_prefix)"
                  >
                    撤销
                  </n-button>
                  <n-popconfirm @positive-click="handleDeleteInvite(inv.id_prefix)">
                    <template #trigger>
                      <n-button
                        size="tiny"
                        type="error"
                        secondary
                        :disabled="!store.isAdmin"
                      >
                        删除
                      </n-button>
                    </template>
                    确定要从集群状态中彻底删除此邀请记录吗？
                  </n-popconfirm>
                </n-space>
              </td>
            </tr>
          </tbody>
        </n-table>
      </n-scrollbar>
    </n-card>

    <!-- Create Invite Modal -->
    <n-modal v-model:show="showInviteModal" preset="card" title="创建 Hub 入网邀请 (Invite)" style="width: 480px; max-width: calc(100vw - 32px);">
      <n-form label-placement="left" label-width="120">
        <n-form-item label="Hub 节点 ID">
          <n-input v-model:value="inviteForm.member_id" placeholder="如 hub-backup2" />
        </n-form-item>
        <n-form-item label="预设优先级">
          <n-input-number v-model:value="inviteForm.priority" :min="1" :max="100" />
        </n-form-item>
        <n-form-item label="有效期">
          <n-select
            v-model:value="inviteForm.expires"
            :options="[
              { label: '10 分钟', value: '10m' },
              { label: '1 小时', value: '1h' },
              { label: '24 小时', value: '24h' }
            ]"
          />
        </n-form-item>
      </n-form>

      <div v-if="generatedToken" class="generated-token-box">
        <div class="text-muted font-sm mb-1">请复制下方 Token 在待入网 Hub 上执行 Join:</div>
        <n-input type="textarea" :value="generatedToken" readonly rows="3" />
      </div>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showInviteModal = false">关闭</n-button>
          <n-button type="primary" :loading="creatingInvite" @click="handleCreateInvite">
            生成 Invite Token
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Join Cluster Modal -->
    <n-modal v-model:show="showJoinModal" preset="card" title="节点加入 HA 集群 (Join)" style="width: 520px; max-width: calc(100vw - 32px);">
      <n-form label-placement="left" label-width="120">
        <n-form-item label="GRE 隧道接口">
          <n-input v-model:value="joinForm.interface" placeholder="如 gre4-tun0" />
        </n-form-item>
        <n-form-item label="宣告外网地址">
          <n-input v-model:value="joinForm.advertised" placeholder="多个地址用英文逗号隔开，如 114.28.143.35,172.29.1.250" />
        </n-form-item>
        <n-form-item label="Invite Token">
          <n-input
            type="textarea"
            v-model:value="joinForm.token"
            placeholder="粘贴来自 Primary Hub 的 Invite Token 字符串"
            rows="4"
          />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showJoinModal = false">取消</n-button>
          <n-button type="info" :loading="joining" @click="handleJoin">
            确认加入集群
          </n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- Edit Priority Modal -->
    <n-modal
      v-model:show="showPriorityModal"
      preset="card"
      :title="`修改节点优先级: ${editingMember?.member_id}`"
      style="width: 440px; max-width: calc(100vw - 32px);"
      :bordered="false"
    >
      <n-form label-placement="left" label-width="90">
        <n-form-item label="节点 ID">
          <n-input :value="editingMember?.member_id" disabled />
        </n-form-item>
        <n-form-item label="当前角色">
          <n-tag :type="editingMember?.is_leader ? 'success' : 'default'" size="small" round>
            <n-icon><component :is="editingMember?.is_leader ? TrophyOutline : ShieldOutline" /></n-icon>
            {{ editingMember?.is_leader ? 'Leader 主节点' : 'Standby 备用节点' }}
          </n-tag>
        </n-form-item>
        <n-form-item label="新优先级">
          <n-input-number
            v-model:value="priorityForm.priority"
            :min="1"
            :max="100"
            style="width: 100%;"
            placeholder="请输入 1-100（数值越大越优先成为 Leader）"
          />
        </n-form-item>
      </n-form>
      <div class="text-muted font-sm mb-3 info-tip">
        <n-icon><InformationCircleOutline /></n-icon>
        <span>提示：优先级数值决定集群故障选举和回切的主备顺序（通常主节点设为 100，备节点设为 90）。</span>
      </div>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showPriorityModal = false">取消</n-button>
          <n-button type="primary" :loading="updatingPriority" @click="handleSavePriority">
            保存修改
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import {
  NGrid,
  NGridItem,
  NCard,
  NButton,
  NTable,
  NTag,
  NSpace,
  NPopconfirm,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NScrollbar,
  NSkeleton,
  NIcon,
  createDiscreteApi,
  darkTheme,
} from 'naive-ui'
import {
  CheckmarkCircleOutline,
  CloseCircleOutline,
  InformationCircleOutline,
  ShieldOutline,
  TrophyOutline,
} from '@vicons/ionicons5'
import { api } from '../api/client'
import { useAppStore } from '../store'
import type { ClusterStatus, ReplicationStatus, MemberInfo, InviteRecord, KeyStatus } from '../types'

const store = useAppStore()
const { message } = createDiscreteApi(['message'], {
  configProviderProps: computed(() => ({
    theme: store.isDark ? darkTheme : null,
  })),
})

const loading = ref(true)
const cluster = ref<ClusterStatus | null>(null)
const replication = ref<ReplicationStatus | null>(null)
const members = ref<MemberInfo[]>([])
const invites = ref<InviteRecord[]>([])
const keyStatus = ref<KeyStatus | null>(null)
const clusterServiceAvailable = computed(() => Boolean(
  cluster.value?.service_available &&
  !cluster.value?.isolated &&
  (cluster.value?.witness?.quorum_available ?? true),
))

const clusterTargetNodeId = computed(() => {
  const reportedLeader = store.nodes.find((node) =>
    node.status === 'online' &&
    (node.id === cluster.value?.leader || node.name === cluster.value?.leader)
  )
  if (reportedLeader) return reportedLeader.id

  const leaderNode = store.nodes.find((node) => node.role === 'leader' && node.status === 'online')
  if (leaderNode) return leaderNode.id

  const onlineHub = store.nodes.find((node) => node.status === 'online' && node.id !== 'local')
  if (onlineHub) return onlineHub.id

  return store.activeNodeId || store.nodes[0]?.id || ''
})

const haOnlineCount = computed(() => members.value.filter(isMemberHAOnline).length)
const networkHealthText = computed(() => {
  switch (cluster.value?.network_health_status) {
    case 'healthy': return '网络健康'
    case 'unhealthy': return '网络异常'
    case 'disabled': return '探测未启用'
    default: return '网络未知'
  }
})
const networkHealthType = computed(() => {
  switch (cluster.value?.network_health_status) {
    case 'healthy': return 'success'
    case 'unhealthy': return 'error'
    case 'disabled': return 'default'
    default: return 'warning'
  }
})

const showInviteModal = ref(false)
const showJoinModal = ref(false)
const showPriorityModal = ref(false)
const updatingPriority = ref(false)
const editingMember = ref<MemberInfo | null>(null)
const priorityForm = ref({
  priority: 100,
})

const creatingInvite = ref(false)
const joining = ref(false)
const generatedToken = ref('')

const inviteForm = ref({
  member_id: '',
  priority: 90,
  expires: '10m',
})

const joinForm = ref({
  interface: 'gre4-tun0',
  advertised: '',
  token: '',
})

function isMemberHAOnline(member: MemberInfo) {
	if (member.state === 'disabled') return false
	const isSelf = member.member_id === cluster.value?.member
	return isSelf
		? clusterServiceAvailable.value
		: member.connected && member.authenticated
}

function memberHAStatus(member: MemberInfo) {
	if (member.state === 'disabled') return '已禁用'
	const isSelf = member.member_id === cluster.value?.member
	const isLeader = member.member_id === cluster.value?.leader || member.is_leader
	if (isSelf) {
		if (cluster.value?.isolated) {
			const witness = cluster.value.witness
			if (witness && witness.mode !== 'legacy' && !witness.quorum_available) {
				return witness.mode === 'active' ? '已隔离 (无有效仲裁)' : `已隔离 (Witness ${witness.mode})`
			}
			return '已隔离'
		}
		if (!clusterServiceAvailable.value) return '服务不可用 (本机)'
		return isLeader ? '在线 (Leader / 本机)' : '在线 (本机)'
	}
	if (member.connected && member.authenticated) return isLeader ? '在线 (Leader / 已认证)' : '在线 (已认证同步)'
  if (member.connected) return '连接未认证'
  return 'HA 会话中断'
}

function managerNode(member: MemberInfo) {
  return store.nodes.find((node) =>
    node.id === member.member_id ||
    node.name === member.member_id ||
    (!!node.advertised_ip && member.advertised_addresses?.includes(node.advertised_ip))
  )
}

function formatLastSeen(lastSeen?: string) {
  if (!lastSeen) return '-'
  const date = new Date(lastSeen)
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString()
}

function shortValue(value?: string, length = 16) {
  if (!value) return '-'
  return value.length > length ? `${value.slice(0, length)}…` : value
}

function replicationDigest(memberId: string) {
  if (memberId === cluster.value?.member) return replication.value?.digest
  return replication.value?.peers.find((peer) => peer.member_id === memberId)?.digest
}

const loadData = async () => {
  try {
    await store.fetchNodes()
    const targetNode = clusterTargetNodeId.value
    if (!targetNode) return

    const [c, r, inv, k] = await Promise.all([
      api.getClusterStatus(targetNode),
      api.getReplicationStatus(targetNode),
      api.listInvites(targetNode),
      api.getKeyStatus(targetNode),
    ])
    cluster.value = c
    replication.value = r
    members.value = c.members || []
    invites.value = inv
    keyStatus.value = k
  } catch (e) {
    console.error('Load HA data error', e)
  } finally {
    loading.value = false
  }
}

const handleCreateInvite = async () => {
  if (!inviteForm.value.member_id) {
    message.error('请输入 Member ID')
    return
  }
  creatingInvite.value = true
  try {
    const res = await api.createInvite(clusterTargetNodeId.value, inviteForm.value)
    generatedToken.value = res.invite_token
    message.success('Invite 令牌创建成功')
    loadData()
  } catch (e: any) {
    message.error(e.response?.data?.error || '创建 Invite 失败')
  } finally {
    creatingInvite.value = false
  }
}

const handleRevokeInvite = async (idPrefix: string) => {
  try {
    await api.revokeInvite(clusterTargetNodeId.value, idPrefix)
    message.success('已撤销邀请')
    loadData()
  } catch (e: any) {
    message.error(e.response?.data?.error || '撤销失败')
  }
}

const handleDeleteInvite = async (idPrefix: string) => {
  try {
    await api.deleteInvite(clusterTargetNodeId.value, idPrefix)
    message.success('已从集群状态中删除此邀请记录')
    loadData()
  } catch (e: any) {
    message.error(e.response?.data?.error || '删除失败')
  }
}

const handleJoin = async () => {
  if (!joinForm.value.token) {
    message.error('请填写 Invite Token')
    return
  }
  joining.value = true
  try {
    const advertisedArr = joinForm.value.advertised
      ? joinForm.value.advertised.split(',').map((s) => s.trim())
      : []
    await api.joinCluster(clusterTargetNodeId.value, {
      invite_token: joinForm.value.token,
      interface: joinForm.value.interface,
      advertised_addresses: advertisedArr,
    })
    message.success('成功加入 HA 集群！')
    showJoinModal.value = false
    loadData()
  } catch (e: any) {
    message.error(e.response?.data?.error || '加入集群失败')
  } finally {
    joining.value = false
  }
}

const handleFailback = async () => {
  try {
    await api.requestFailback(clusterTargetNodeId.value, true)
    message.success('已下发强制回切 Primary 请求')
    loadData()
  } catch (e: any) {
    message.error('请求回切失败')
  }
}

const handleRotateKey = async (action: 'prepare' | 'commit') => {
  try {
    await api.rotateKey(clusterTargetNodeId.value, action)
    message.success(`密钥轮转 ${action} 成功`)
    loadData()
  } catch (e: any) {
    message.error('密钥操作失败: ' + (e.response?.data?.error || ''))
  }
}

const handleSetMemberDisabled = async (memberId: string, disabled: boolean) => {
  try {
    await api.setMember(clusterTargetNodeId.value, { member_id: memberId, disabled })
    message.success(`已${disabled ? '禁用' : '启用'}节点 ${memberId}`)
    loadData()
  } catch (e: any) {
    message.error(e.response?.data?.error || '操作失败')
  }
}

const openEditPriority = (m: MemberInfo) => {
  editingMember.value = m
  priorityForm.value = {
    priority: m.priority || 100,
  }
  showPriorityModal.value = true
}

const handleSavePriority = async () => {
  if (!editingMember.value) return
  updatingPriority.value = true
  try {
    await api.setMember(clusterTargetNodeId.value, {
      member_id: editingMember.value.member_id,
      priority: priorityForm.value.priority,
    })
    message.success(`节点 ${editingMember.value.member_id} 优先级已更新为 ${priorityForm.value.priority}`)
    showPriorityModal.value = false
    loadData()
  } catch (e: any) {
    message.error(e.response?.data?.error || '优先级更新失败')
  } finally {
    updatingPriority.value = false
  }
}

let timer: number | null = null

onMounted(() => {
  loadData()
  timer = window.setInterval(loadData, 3000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
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

.mb-4 {
  margin-bottom: 16px;
}

.cluster-summary-card {
  height: 100%;
  min-height: 118px;
  max-height: 118px;
  display: flex;
  flex-direction: column;
  background: var(--bg-card) !important;
  border: 1px solid var(--border-color) !important;
  border-radius: 8px;
  box-shadow: var(--card-shadow);
  transition: background-color 0.2s ease, border-color 0.2s ease;
  overflow: hidden;
  box-sizing: border-box;
}

.cluster-summary-card :deep(.n-card__content) {
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  height: 100%;
  padding: 10px 14px !important;
  box-sizing: border-box;
  overflow: hidden;
}

.card-header-label {
  font-size: 12px;
  line-height: 16px;
  height: 16px;
  font-weight: 500;
  color: var(--text-muted);
  white-space: nowrap;
}

.card-large-text {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-title);
  height: 24px;
  min-height: 24px;
  line-height: 24px;
  margin: 2px 0 4px 0;
  display: flex;
  align-items: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.card-meta-list {
  display: flex;
  flex-direction: column;
  gap: 3px;
  height: 42px;
  min-height: 42px;
  max-height: 42px;
  justify-content: flex-end;
  overflow: hidden;
}

.tag-row {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 4px;
  height: 20px;
  min-height: 20px;
  overflow: hidden;
}

.sub-info {
  font-size: 11px;
  line-height: 16px;
  height: 16px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  display: flex;
  align-items: center;
}

.action-row {
  display: flex;
  align-items: center;
  gap: 6px;
  height: 18px;
}

.divider {
  color: var(--border-color);
  font-size: 11px;
}

.text-emerald {
  color: #10b981;
}

.text-amber {
  color: #f59e0b;
}

.text-muted {
  color: var(--text-muted);
}

.download-link {
  color: #10b981;
  text-decoration: none;
}

.download-link:hover {
  text-decoration: underline;
}

.generated-token-box {
  margin-top: 16px;
  padding: 12px;
  background: var(--bg-card-secondary);
  border: 1px solid var(--border-color);
  border-radius: 6px;
}

.font-sm {
  font-size: 12px;
}

.text-center {
  text-align: center;
}

.icon-value,
.target-item,
.info-tip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  vertical-align: middle;
}

.mr-1 {
  margin-right: 4px;
}

.ml-2 {
  margin-left: 8px;
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
  .page-header .n-button {
    width: 100% !important;
    justify-content: center !important;
  }
  .card-large-text {
    font-size: 17px;
  }
}
</style>
