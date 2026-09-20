import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import type { MemberInfo, NodeRecord } from '../src/types/index.ts'
import {
  classifyHALink,
  classifyHubStatus,
  findLatestLeaderNode,
  formatHubStatus,
  formatWitnessStatus,
  isHubNode,
  isNodeSelectable,
  selectActiveNode,
  splitBalanced,
  type HubStatusContext,
} from '../src/utils/topologyStatus.ts'

const member = (id: string, connected = false): MemberInfo => ({
  member_id: id,
  priority: id === 'hub-primary' ? 100 : id === 'hub-backup1' ? 90 : 80,
  state: 'active',
  is_leader: id === 'hub-primary',
  connected,
  authenticated: connected,
})
const node = (id: string, status: NodeRecord['status'] = 'online'): NodeRecord => ({
  id,
  name: id,
  type: 'hub',
  host: id,
  status,
  role: id === 'hub-primary' ? 'leader' : 'standby',
  term: 10,
  service_avail: true,
  last_seen: '',
})
const context = (selectedMemberId: string): HubStatusContext => ({
  selectedMemberId,
  leaderMemberId: 'hub-primary',
  primaryMemberId: 'hub-primary',
  term: 10,
})

// A third Standby's view cannot observe the other Standby's HA session.
const leaderFromBackup2 = classifyHubStatus(member('hub-primary', true), node('hub-primary'), undefined, context('hub-backup2'))
const backup1FromBackup2 = classifyHubStatus(member('hub-backup1'), node('hub-backup1'), undefined, context('hub-backup2'))
assert.equal(backup1FromBackup2.session, 'unknown')
assert.equal(classifyHALink(leaderFromBackup2, backup1FromBackup2, 'hub-backup2'), 'online')
assert.doesNotMatch(formatHubStatus(backup1FromBackup2, 90), /HA会话中断/)

// A Leader directly observes a disconnected Standby session.
const leaderSelf = classifyHubStatus(member('hub-primary'), node('hub-primary'), undefined, context('hub-primary'))
const backup1FromLeader = classifyHubStatus(member('hub-backup1'), node('hub-backup1'), undefined, context('hub-primary'))
assert.equal(classifyHALink(leaderSelf, backup1FromLeader, 'hub-primary'), 'disconnected')
assert.match(formatHubStatus(backup1FromLeader, 90), /HA会话中断/)

const follower = classifyHubStatus(member('hub-backup1'), { ...node('hub-backup1'), role: 'follower' }, undefined, context('hub-primary'))
assert.equal(follower.state, 'follower')
assert.equal(follower.isFollower, true)
assert.match(formatHubStatus(follower, 90), /Follower/)

// Agent liveness is an overlay: a live HA session does not become a node outage.
const backup1AgentOffline = classifyHubStatus(member('hub-backup1', true), node('hub-backup1', 'offline'), undefined, context('hub-primary'))
assert.equal(backup1AgentOffline.state, 'standby')
assert.equal(backup1AgentOffline.isAgentOffline, true)
assert.equal(backup1AgentOffline.isOffline, false)

const isolatedContext = { ...context('hub-primary'), clusterIsolated: true }
const isolatedPrimary = classifyHubStatus(member('hub-primary'), node('hub-primary'), undefined, isolatedContext)
const witnessTransition = {
  mode: 'preparing', policy: 'manager-witness', transition: 'waiting_hubs',
  decision_reason: '等待两台 Hub 收敛', members: [],
} as any
assert.match(formatHubStatus(isolatedPrimary, 100, witnessTransition), /Witness 切换中.*等待两台 Hub 收敛/)
witnessTransition.mode = 'active'
witnessTransition.decision_reason = 'lease refused'
assert.match(formatHubStatus(isolatedPrimary, 100, witnessTransition), /无有效仲裁.*lease refused/)

const disabled = member('hub-backup1')
disabled.state = 'disabled'
assert.equal(classifyHubStatus(disabled, node('hub-backup1', 'offline'), undefined, context('hub-primary')).state, 'disabled')
assert.equal(formatWitnessStatus({ mode: 'legacy', policy: 'hub-majority', quorum_available: true, required: 2 } as any, 3), 'Hub majority · 3 online · 需 2')
assert.equal(isHubNode(node('hub-primary')), true)
assert.equal(isHubNode({ ...node('branch-1'), type: 'spoke', role: 'spoke' }), false)
assert.deepEqual(splitBalanced(['H2', 'H3']), [['H2'], ['H3']])

const oldLeader = node('hub-primary')
const oldStandby = node('hub-backup1')
assert.equal(selectActiveNode([oldLeader, oldStandby], oldStandby.id, oldLeader.id).activeNodeId, oldStandby.id)
oldLeader.role = 'standby'
oldStandby.role = 'leader'
assert.equal(selectActiveNode([oldLeader, oldStandby], oldLeader.id, oldLeader.id).activeNodeId, oldStandby.id)

oldLeader.role = 'isolated'
oldLeader.service_avail = false
oldStandby.term = 11
assert.equal(selectActiveNode([oldLeader, oldStandby], oldLeader.id, oldStandby.id).activeNodeId, oldLeader.id)

const spokesView = readFileSync(new URL('../src/views/Spokes.vue', import.meta.url), 'utf8')
const hubHAView = readFileSync(new URL('../src/views/HubHA.vue', import.meta.url), 'utf8')
const configView = readFileSync(new URL('../src/views/ConfigEditor.vue', import.meta.url), 'utf8')
const auditView = readFileSync(new URL('../src/views/Audit.vue', import.meta.url), 'utf8')
const witnessView = readFileSync(new URL('../src/views/WitnessSLA.vue', import.meta.url), 'utf8')
const usersView = readFileSync(new URL('../src/views/UserManagement.vue', import.meta.url), 'utf8')
const terminalLog = readFileSync(new URL('../src/components/TerminalLog.vue', import.meta.url), 'utf8')
const topologyGraph = readFileSync(new URL('../src/components/TopologyGraph.vue', import.meta.url), 'utf8')
assert.match(spokesView, /row\.managed\.status === 'online'/)
assert.match(spokesView, /:disabled="!store\.isAdmin"/)
assert.match(spokesView, /haStatus\.selection_mode === 'manual'/)
assert.match(spokesView, /api\.setManagedSpokeHAMode/)
assert.match(spokesView, /设为手动目标/)
assert.match(spokesView, /@click="openQuickRegister\(row\.spoke!\)"/)
assert.match(spokesView, /api\.createManagedSpoke\(registerForm\.value\)/)
assert.match(spokesView, /api\.rotateManagedSpokeToken\(registerTarget\.value, registerSource\.value\.protocol_address\)/)
assert.match(spokesView, /spoke\.managed_node_id/)
assert.match(spokesView, /store\.topologySnapshot/)
assert.match(spokesView, /spokes_by_node/)
assert.match(spokesView, /window\.setInterval\(\(\) => \{ void refreshOpenHAStatus\(\) \}, 3000\)/)
assert.match(spokesView, /watch\(showManage/)
assert.match(spokesView, /onBeforeUnmount\(stopHARefresh\)/)
assert.doesNotMatch(spokesView, /api\.listSpokeOverview\(\)/)
assert.doesNotMatch(spokesView, /api\.listSpokes\(/)
assert.doesNotMatch(spokesView, /currentHubs\.map\(\(hub\) => api\.listSpokes\(hub\.id\)\)/)
assert.doesNotMatch(spokesView, /store\.liveRevision/)
assert.match(spokesView, /所属 Hub/)
assert.match(spokesView, /Token 只显示这一次/)
assert.match(hubHAView, /store\.topologySnapshot/)
assert.match(hubHAView, /snapshot\.replication/)
assert.doesNotMatch(hubHAView, /store\.liveRevision/)
assert.doesNotMatch(hubHAView, /api\.(getClusterStatus|getReplicationStatus|listInvites|getKeyStatus)\(/)
assert.doesNotMatch(configView, /store\.liveRevision/)
assert.doesNotMatch(configView, /api\.getAuditLogs\(/)
assert.doesNotMatch(configView, /OpenNHRP 进程操作/)
assert.doesNotMatch(configView, /saveComment/)
assert.match(configView, /保存配置文件 \(Save\)/)
assert.match(configView, /class="editor-actions"/)
assert.match(configView, /n-card-content/)
assert.match(configView, /添加静态映射[\s\S]*保存 Map[\s\S]*清除重定向缓存[\s\S]*热重载配置 \(Reload\)[\s\S]*保存配置文件 \(Save\)/)
assert.match(auditView, /api\.getAuditLogs\(50\)/)
assert.doesNotMatch(auditView, /store\.liveRevision/)
assert.doesNotMatch(witnessView, /store\.liveRevision/)
assert.doesNotMatch(usersView, /store\.liveRevision/)
assert.match(terminalLog, /props\.nodeId && data\.node_id !== props\.nodeId/)
assert.match(topologyGraph, /const selectedSpokeHub = hubNodes\.find\(\(h\) => h\.memberId === localMemberId\)/)
assert.doesNotMatch(topologyGraph, /h\.hubStatus\?\.isLeader && h\.hubStatus\.isOnline/)
const staleLeader = node('hub-primary')
staleLeader.term = 10
staleLeader.role = 'leader'
assert.equal(findLatestLeaderNode([staleLeader, oldStandby])?.id, oldStandby.id)
assert.equal(isNodeSelectable(oldLeader), true)
oldLeader.status = 'offline'
assert.equal(isNodeSelectable(oldLeader), false)
assert.equal(selectActiveNode([oldLeader, oldStandby], oldLeader.id, oldStandby.id).activeNodeId, oldLeader.id)
