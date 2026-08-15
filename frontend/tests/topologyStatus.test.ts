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

const managedSpokesView = readFileSync(new URL('../src/views/ManagedSpokes.vue', import.meta.url), 'utf8')
const spokesView = readFileSync(new URL('../src/views/Spokes.vue', import.meta.url), 'utf8')
const terminalLog = readFileSync(new URL('../src/components/TerminalLog.vue', import.meta.url), 'utf8')
assert.match(managedSpokesView, /spoke\.status === 'online'/)
assert.match(managedSpokesView, /:disabled="!store\.isAdmin"/)
assert.match(spokesView, /@click="openQuickRegister\(s\)"/)
assert.match(spokesView, /api\.createManagedSpoke\(quickForm\.value\)/)
assert.match(spokesView, /api\.rotateManagedSpokeToken\(quickTarget\.value, quickSource\.value\.protocol_address\)/)
assert.match(spokesView, /s\.managed_node_id/)
assert.match(spokesView, /protocol_address: s\.protocol_address/)
assert.match(spokesView, /Token 只显示这一次/)
assert.match(terminalLog, /props\.nodeId && data\.node_id !== props\.nodeId/)
const staleLeader = node('hub-primary')
staleLeader.term = 10
staleLeader.role = 'leader'
assert.equal(findLatestLeaderNode([staleLeader, oldStandby])?.id, oldStandby.id)
assert.equal(isNodeSelectable(oldLeader), true)
oldLeader.status = 'offline'
assert.equal(isNodeSelectable(oldLeader), false)
assert.equal(selectActiveNode([oldLeader, oldStandby], oldLeader.id, oldStandby.id).activeNodeId, oldLeader.id)
