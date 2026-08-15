import type { MemberInfo, NodeRecord, SLAMatrixItem, WitnessQuorumStatus, WitnessStatus } from '../types'

export type HASessionState = 'connected' | 'disconnected' | 'self' | 'unknown'
export type HubDisplayState = 'disabled' | 'offline' | 'isolated' | 'learner' | 'leader' | 'standby'
export type HALinkState = 'online' | 'disconnected' | 'unknown' | 'disabled'

export interface HubStatusContext {
  selectedMemberId?: string
  leaderMemberId?: string
  primaryMemberId?: string
  term?: number
  clusterIsolated?: boolean
  stale?: boolean
}

export interface HubTopologyStatus {
  memberId: string
  state: HubDisplayState
  session: HASessionState
  isPrimary: boolean
  isLeader: boolean
  isOnline: boolean
  isDisabled: boolean
  isOffline: boolean
  isIsolated: boolean
  isLearner: boolean
  isAgentOnline: boolean
  isAgentOffline: boolean
  termMismatch: boolean
}

export function splitBalanced<T>(items: T[]): [T[], T[]] {
  const middle = Math.ceil(items.length / 2)
  return [items.slice(0, middle), items.slice(middle)]
}

export function isNodeSelectable(node: NodeRecord | undefined): boolean {
  return Boolean(node && node.status !== 'offline')
}

export function isHubNode(node: NodeRecord): boolean {
  return node.id !== 'local' && node.type !== 'local' && node.type !== 'spoke' && node.role !== 'witness'
}

export function findLatestLeaderNode(nodes: NodeRecord[]): NodeRecord | undefined {
  return nodes
    .filter((node) => isNodeSelectable(node) && node.role === 'leader')
    .reduce<NodeRecord | undefined>((latest, node) => !latest || node.term > latest.term ? node : latest, undefined)
}

export function selectActiveNode(
  nodes: NodeRecord[],
  activeNodeId: string,
  previousLeaderId: string,
): { activeNodeId: string; leaderNodeId: string } {
  const leader = findLatestLeaderNode(nodes)
  const selectableNode = nodes.find(isNodeSelectable)
  const activeNode = nodes.find((node) => node.id === activeNodeId)
  const leaderChanged = Boolean(leader && leader.id !== previousLeaderId)
  const leaderNodeId = leader?.id || previousLeaderId

  if (leader && leaderChanged) {
    return { activeNodeId: leader.id, leaderNodeId }
  }
  if (!activeNode) {
    return { activeNodeId: leader?.id || selectableNode?.id || nodes[0]?.id || '', leaderNodeId }
  }
  return { activeNodeId, leaderNodeId }
}

const sessionState = (
  member: MemberInfo,
  context: HubStatusContext,
): HASessionState => {
  if (context.stale) return 'unknown'
  if (member.member_id === context.selectedMemberId) return 'self'

  const isDirectPeer = context.selectedMemberId === context.leaderMemberId ||
    member.member_id === context.leaderMemberId
  if (!isDirectPeer) return 'unknown'

  return member.connected && member.authenticated ? 'connected' : 'disconnected'
}

export function classifyHubStatus(
  member: MemberInfo,
  node: NodeRecord | undefined,
  sla: SLAMatrixItem | undefined,
  context: HubStatusContext,
): HubTopologyStatus {
  const session = sessionState(member, context)
  const isDisabled = member.state === 'disabled'
  const isAgentOnline = node?.status === 'online' || node?.status === 'degraded'
  const isAgentOffline = Boolean(node) && !isAgentOnline
  const termMismatch = Boolean(node?.term && context.term && node.term !== context.term)
  const isLearner = member.state === 'learner' || node?.role === 'learner'
  const isSelf = member.member_id === context.selectedMemberId
  const isIsolated = !isDisabled && (
    node?.role === 'isolated' ||
    (isAgentOnline && node?.service_avail === false) ||
    (isSelf && context.clusterIsolated === true)
  )
  const slaOffline = Boolean(
    sla &&
    (!sla.l4_healthy || sla.overall_state === 'critical') &&
    !sla.firewall_protected &&
    sla.agent_healthy !== true,
  )
  const isOffline = !isDisabled && !isIsolated && session === 'disconnected' &&
    (isAgentOffline || (!node && slaOffline))
  const isLeader = member.member_id === context.leaderMemberId
  const state: HubDisplayState = isDisabled
    ? 'disabled'
    : isOffline
      ? 'offline'
      : isIsolated
        ? 'isolated'
        : isLearner
          ? 'learner'
          : isLeader
            ? 'leader'
            : 'standby'

  return {
    memberId: member.member_id,
    state,
    session,
    isPrimary: member.member_id === context.primaryMemberId,
    isLeader,
    isOnline: !isDisabled && !isOffline && !isIsolated,
    isDisabled,
    isOffline,
    isIsolated,
    isLearner,
    isAgentOnline,
    isAgentOffline,
    termMismatch,
  }
}

export function classifyHALink(
  leader: HubTopologyStatus,
  standby: HubTopologyStatus,
  selectedMemberId?: string,
): HALinkState {
  if (leader.isDisabled || standby.isDisabled) return 'disabled'
  if (leader.isOffline || standby.isOffline || leader.isIsolated || standby.isIsolated) {
    return 'disconnected'
  }

  const directSession = selectedMemberId === leader.memberId
    ? standby.session
    : selectedMemberId === standby.memberId
      ? leader.session
      : 'unknown'
  if (directSession === 'connected') return 'online'
  if (directSession === 'disconnected') return 'disconnected'
  if (leader.isLearner || standby.isLearner || leader.termMismatch || standby.termMismatch) {
    return 'unknown'
  }
  return leader.isAgentOnline && standby.isAgentOnline ? 'online' : 'unknown'
}

function witnessProblem(quorum: WitnessQuorumStatus | null | undefined): string {
  if (!quorum) return ''
  const reason = quorum.decision_reason ? `：${quorum.decision_reason}` : ''
  if (quorum.policy === 'hub-majority') {
    return isWitnessQuorumHealthy(quorum) ? '' : `Hub majority 不可用${reason}`
  }
  if (quorum.mode !== 'active' && quorum.mode !== 'legacy') {
    return `Witness 切换中 (${quorum.transition || quorum.mode})${reason}`
  }
  return quorum.mode === 'active' && !isWitnessQuorumHealthy(quorum)
    ? `无有效仲裁${reason}`
    : ''
}

export function formatHubStatus(
  status: HubTopologyStatus,
  priority: number,
  quorum?: WitnessQuorumStatus | null,
): string {
  const role = status.isPrimary ? '主 Hub' : '备 Hub'
  if (status.isDisabled) return `${role} 已禁用 (Disabled / Pri: ${priority})`
  if (status.isOffline) return `${role} 离线 (${status.isPrimary ? 'Primary' : 'Backup'} Offline)`
  if (status.isIsolated) return `${role} 已隔离 (${witnessProblem(quorum) || 'Isolated'} / Pri: ${priority})`
  if (status.isLearner) return `${role} 同步中 (Learner / Pri: ${priority})`

  const details = [status.isLeader ? 'Leader' : 'Standby']
  if (status.session === 'disconnected') details.push('HA会话中断')
  if (status.termMismatch) details.push('Term未收敛')
  if (status.isAgentOffline) details.push('Agent离线')
  details.push(`Pri: ${priority}`)
  const takeover = status.isLeader && !status.isPrimary ? ' 接管' : ''
  return `${role}${takeover} (${details.join(' / ')})`
}

export function formatWitnessStatus(witness: WitnessStatus | undefined, onlineHubs: number): string {
  if (!witness) return 'Legacy availability-first'
  if (witness.policy === 'hub-majority') {
    const state = witness.quorum_available ? 'Hub majority' : 'Hub majority不可用'
    return `${state} · ${onlineHubs} online · 需 ${witness.required}`
  }
  if (witness.mode === 'legacy') return 'Legacy availability-first'
  if (witness.mode === 'active') {
    return witness.quorum_available
      ? `Manager Witness · ${witness.votes}/${witness.required} · ${witness.lease_holder || 'peer vote'} · ${(witness.lease_remaining_ms / 1000).toFixed(1)}s`
      : 'Manager Witness · 无多数派 · 已隔离'
  }
  return `Manager Witness 转换中 · ${witness.mode}`
}

export function isWitnessQuorumHealthy(quorum: WitnessQuorumStatus | null | undefined): boolean {
  if (!quorum) return false
  if (quorum.policy !== 'hub-majority' && quorum.mode === 'legacy') return true
  return quorum.members.some((member) => member.fresh && member.quorum_available && !member.fenced)
}

export function formatWitnessQuorumStatus(quorum: WitnessQuorumStatus | null | undefined): string {
  if (!quorum) return '全局仲裁状态未知'
  if (quorum.policy === 'hub-majority') {
    const onlineHubs = quorum.members.filter((member) => member.agent_connected && member.fresh).length
    const state = isWitnessQuorumHealthy(quorum) ? 'Hub majority' : witnessProblem(quorum)
    return `${state} · ${onlineHubs} online · 需 ${quorum.required}`
  }
  if (quorum.mode === 'legacy') return 'Legacy availability-first'
  if (quorum.mode === 'active') {
    return isWitnessQuorumHealthy(quorum)
      ? `Manager Witness · ${quorum.votes}/${quorum.voters} · ${quorum.holder || 'peer vote'} · ${(quorum.lease_remaining_ms / 1000).toFixed(1)}s`
      : `Manager Witness · ${witnessProblem(quorum)}`
  }
  return witnessProblem(quorum)
}
