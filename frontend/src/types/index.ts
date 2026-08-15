export interface ClusterStatus {
  stale?: boolean
  action?: string
  cluster_id?: string
  primary?: string
  member: string
  interface?: string
  protocol_address?: string
  prefix_length?: number
  state_dir?: string
  term: number
  commit_index: number
  leader: string
  local_role: 'leader' | 'standby' | 'learner' | 'isolated' | 'standalone'
  manifest_revision: number
  digest?: string
  service_available: boolean
  network_health: boolean
  network_health_status: 'healthy' | 'unhealthy' | 'disabled' | 'unknown'
  health_interval_seconds: number
  health_failure_rounds: number
  health_recovery_rounds: number
  isolated: boolean
  failback_pending?: boolean
  failback_deadline?: string
  health_targets?: HealthTargetInfo[]
  members?: MemberInfo[]
  witness?: WitnessStatus
}

export interface WitnessStatus {
  capable: boolean
  mode: 'legacy' | 'preparing' | 'active' | 'disabling'
  policy: 'legacy' | 'manager-witness' | 'hub-majority'
  voters: number
  required: number
  votes: number
  epoch: string
  peer_vote: boolean
  manager_vote: boolean
  quorum_available: boolean
  lease_holder: string
  lease_term: number
  lease_sequence: number
  lease_remaining_ms: number
  fallback_remaining_ms: number
  digest?: string
}

export interface WitnessQuorumMember {
  node_id: string
  member_id: string
  agent_connected: boolean
  fresh: boolean
  role: string
  term: number
  commit_index: number
  digest: string
  peer_vote: boolean
  manager_vote: boolean
  quorum_available: boolean
  fenced: boolean
}

export interface WitnessQuorumStatus {
  cluster_id: string
  mode: 'legacy' | 'preparing' | 'active' | 'disabling'
  policy: 'legacy' | 'manager-witness' | 'hub-majority'
  epoch: string
  voters: number
  required: number
  votes: number
  leader: string
  term: number
  holder: string
  lease_remaining_ms: number
  fallback_remaining_ms: number
  transition: string
  decision_reason: string
  members: WitnessQuorumMember[]
}

export interface HealthTargetInfo {
  target_ip: string
  last_success: boolean
  consecutive_ok: number
  consecutive_ng: number
  interval_sec: number
  rtt_ms?: number
}

export interface MemberInfo {
  member_id: string
  priority: number
  state: string
  is_leader: boolean
  advertised_addresses?: string[]
  observed_address?: string
  match_index?: number
  lag?: number
  connected: boolean
  authenticated: boolean
  rtt_ms?: number
  digest?: string
}

export interface ReplicationStatus {
  local_index: number
  digest: string
  snapshots_sent: number
  deltas_sent: number
  snapshots_received: number
  deltas_received: number
  resync_requests: number
  peers: ReplicationPeerInfo[]
}

export interface ReplicationPeerInfo {
  member_id: string
  match_index: number
  lag: number
  digest: string
  connected: boolean
}

export interface InviteRecord {
  id_prefix: string
  member_id: string
  priority: number
  state: 'unused' | 'claimed' | 'expired' | 'revoked'
  expires_at: string
}

export interface InviteResult {
  member_id: string
  invite_token: string
  expires_at: string
  priority: number
}

export interface KeyStatus {
  current_key_id: string
  next_key_id?: string
  has_next_key: boolean
}

export interface SpokeInfo {
  stale?: boolean
  protocol_address: string
  nbma_address: string
  nat_address?: string
  interface: string
  type: 'dynamic' | 'direct' | 'shadow' | 'static' | 'local'
  flags: string
  holding_time: number
  expires_in_sec: number
  last_seen?: string
  alias?: string
  site_name?: string
  managed_node_id?: string
  managed_node_name?: string
  managed_status?: 'online' | 'offline' | 'degraded'
}

export interface InterfaceInfo {
  name: string
  type?: string
  protocol_address?: string
  mtu?: number
  nbma_mtu?: number
  nbma_address?: string
  nat_address?: string
  link_name?: string
  flags?: string
  is_up?: boolean
  rx_packets?: number
  tx_packets?: number
  rx_bytes?: number
  tx_bytes?: number
}

export interface NodeRecord {
  id: string
  name: string
  type: string
  host: string
  status: 'online' | 'offline' | 'degraded'
  role: string
  term: number
  advertised_ip?: string
  network_health?: boolean
  service_avail?: boolean
  active_spokes?: number
  ws_rtt_ms?: number
  probe_mode?: 'hybrid' | 'agent_only' | 'active_only'
  last_seen: string
}

export interface ManagedSpoke {
  id: string
  name: string
  status: 'online' | 'offline' | 'degraded'
  host: string
  ws_rtt_ms: number
  core_available: boolean
  peer_count: number
  last_seen: string
  protocol_address?: string
}

export interface SLAMatrixItem {
  node_id: string
  avg_rtt_ms: number
  loss_rate: number
  l3_healthy: boolean
  l4_healthy: boolean
  agent_healthy?: boolean
  data_healthy: boolean
  firewall_protected?: boolean
  latency_source?: 'icmp' | 'ws'
  active_spokes?: number
  overall_state: 'healthy' | 'degraded' | 'critical' | 'unknown'
  last_checked: string
}

export interface TopologySnapshot {
  node_id: string
  nodes: NodeRecord[]
  cluster: ClusterStatus | null
  spokes: SpokeInfo[]
  sla_matrix: SLAMatrixItem[]
  witness_quorum: WitnessQuorumStatus
  timestamp: string
}

export interface ProbeRecord {
  id: number
  target_node_id: string
  probe_type: string
  target_ip: string
  rtt_ms: number
  loss_rate: number
  success: boolean
  detail: string
  recorded_at: string
}

export interface ArbitrationRecord {
  id: number
  term: number
  primary_node_id: string
  backup_node_id: string
  involved_node_ids?: string[]
  decision: string
  reason: string
  recorded_at: string
}

export interface AuditLog {
  id: number
  node_id: string
  operator: string
  action: string
  params: string
  success: boolean
  detail: string
  created_at: string
}

export interface UserInfo {
  id: string
  username: string
  role: 'admin' | 'readonly'
}

export interface UserRecord {
  id: string
  username: string
  role: 'admin' | 'readonly'
  created_at: string
  updated_at: string
}

export interface LoginResponse {
  token: string
  user: UserInfo
}

export interface CreateUserPayload {
  username: string
  password: string
  role: 'admin' | 'readonly'
}

export interface UpdateUserPayload {
  role?: 'admin' | 'readonly'
  password?: string
}

export interface ChangePasswordPayload {
  old_password: string
  new_password: string
}
