import axios from 'axios'
import type {
  ClusterStatus,
  ReplicationStatus,
  MemberInfo,
  InviteRecord,
  InviteResult,
  KeyStatus,
  SpokeInfo,
  InterfaceInfo,
  NodeRecord,
  ManagedSpoke,
  SLAMatrixItem,
  ProbeRecord,
  ArbitrationRecord,
  AuditLog,
  UserInfo,
  UserRecord,
  LoginResponse,
  CreateUserPayload,
  UpdateUserPayload,
  ChangePasswordPayload,
  WitnessQuorumStatus,
} from '../types'

const http = axios.create({
  baseURL: '/api',
  timeout: 10000,
})

// Request Interceptor: Attach JWT Token
http.interceptors.request.use((config) => {
  const token = localStorage.getItem('opennhrp_token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Response Interceptor: Handle 401 Unauthorized
http.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      localStorage.removeItem('opennhrp_token')
      localStorage.removeItem('opennhrp_user')
      if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
        window.location.href = '/login'
      }
    }
    return Promise.reject(error)
  }
)

export const api = {
  // Auth
  login: (data: { username: string; password: string }) =>
    http.post<LoginResponse>('/auth/login', data).then((r) => r.data),

  getMe: () =>
    http.get<UserInfo>('/auth/me').then((r) => r.data),

  logout: () =>
    http.post('/auth/logout').then((r) => r.data),

  changePassword: (data: ChangePasswordPayload) =>
    http.put('/auth/change-password', data).then((r) => r.data),

  // User Management (Admin Only)
  listUsers: () =>
    http.get<UserRecord[]>('/users').then((r) => r.data),

  createUser: (data: CreateUserPayload) =>
    http.post<UserRecord>('/users', data).then((r) => r.data),

  updateUser: (id: string, data: UpdateUserPayload) =>
    http.put(`/users/${id}`, data).then((r) => r.data),

  deleteUser: (id: string) =>
    http.delete(`/users/${id}`).then((r) => r.data),

  // Cluster & HA
  getClusterStatus: (nodeId = '') =>
    http.get<ClusterStatus>(`/cluster/status?node_id=${nodeId}`).then((r) => r.data),

  getReplicationStatus: (nodeId = '') =>
    http.get<ReplicationStatus>(`/cluster/replication?node_id=${nodeId}`).then((r) => r.data),

  getMembers: (nodeId = '') =>
    http.get<MemberInfo[]>(`/cluster/members?node_id=${nodeId}`).then((r) => r.data),

  setMember: (nodeId = '', data: { member_id: string; priority?: number; disabled?: boolean; remove?: boolean }) =>
    http.post(`/cluster/member?node_id=${nodeId}`, data).then((r) => r.data),

  createInvite: (nodeId = '', data: { member_id: string; priority?: number; expires?: string }) =>
    http.post<InviteResult>(`/cluster/invite?node_id=${nodeId}`, data).then((r) => r.data),

  listInvites: (nodeId = '') =>
    http.get<InviteRecord[]>(`/cluster/invites?node_id=${nodeId}`).then((r) => r.data),

  revokeInvite: (nodeId = '', idPrefix: string) =>
    http.post(`/cluster/invite/${idPrefix}/revoke?node_id=${nodeId}`).then((r) => r.data),

  deleteInvite: (nodeId = '', idPrefix: string) =>
    http.delete(`/cluster/invite/${idPrefix}?node_id=${nodeId}`).then((r) => r.data),

  joinCluster: (nodeId = '', data: { invite_token: string; interface: string; advertised_addresses?: string[] }) =>
    http.post(`/cluster/join?node_id=${nodeId}`, data).then((r) => r.data),

  getKeyStatus: (nodeId = '') =>
    http.get<KeyStatus>(`/cluster/keys?node_id=${nodeId}`).then((r) => r.data),

  rotateKey: (nodeId = '', action: 'prepare' | 'commit') =>
    http.post(`/cluster/key/rotate?node_id=${nodeId}`, { action }).then((r) => r.data),

  requestFailback: (nodeId = '', force = false) =>
    http.post(`/cluster/failback?node_id=${nodeId}&force=${force}`).then((r) => r.data),

  // Spokes
  listSpokes: (nodeId = '', iface = '') =>
    http.get<SpokeInfo[]>(`/spokes?node_id=${nodeId}&interface=${iface}`).then((r) => r.data),

  addStaticMap: (nodeId = '', data: { interface: string; protocol_address: string; nbma_address: string; register?: boolean }) =>
    http.post(`/spokes/map?node_id=${nodeId}`, data).then((r) => r.data),

  delStaticMap: (nodeId = '', data: { interface: string; protocol_address: string }) =>
    http.delete(`/spokes/map?node_id=${nodeId}`, { data }).then((r) => r.data),

  saveMap: (nodeId = '', iface = 'gre-ha') =>
    http.post(`/spokes/map/save?node_id=${nodeId}&interface=${iface}`).then((r) => r.data),

  updateNBMA: (nodeId = '', data: { protocol_address: string; nbma_address: string }) =>
    http.post(`/spokes/nbma/update?node_id=${nodeId}`, data).then((r) => r.data),

  purgeRedirect: (nodeId = '', protocol_address = '') =>
    http.post(`/spokes/redirect/purge?node_id=${nodeId}&protocol_address=${protocol_address}`).then((r) => r.data),

  setSpokeMetadata: (data: { protocol_address: string; alias?: string; site_name?: string; contact?: string; notes?: string }) =>
    http.post('/spokes/metadata', data).then((r) => r.data),

  generateSpokeConfig: (data: any) =>
    http.post<{ opennhrp_conf: string; setup_script: string }>('/spokes/provision/generate', data).then((r) => r.data),

  // Managed Spoke devices
  listManagedSpokes: () =>
    http.get<ManagedSpoke[]>('/managed-spokes').then((r) => r.data),

  createManagedSpoke: (data: { id: string; name: string; protocol_address?: string }) =>
    http.post<{ spoke: ManagedSpoke; token: string }>('/managed-spokes', data).then((r) => r.data),

  rotateManagedSpokeToken: (id: string, protocolAddress = '') =>
    http.post<{ token: string }>(`/managed-spokes/${encodeURIComponent(id)}/token/rotate`, protocolAddress ? { protocol_address: protocolAddress } : undefined).then((r) => r.data),

  deleteManagedSpoke: (id: string) =>
    http.delete(`/managed-spokes/${encodeURIComponent(id)}`),

  listManagedSpokePeers: (id: string) =>
    http.get<SpokeInfo[]>(`/managed-spokes/${encodeURIComponent(id)}/peers`).then((r) => r.data),

  // Witness & SLA
  getSLAMatrix: () =>
    http.get<SLAMatrixItem[]>('/witness/sla').then((r) => r.data),

  getWitnessQuorum: () =>
    http.get<WitnessQuorumStatus>('/witness/quorum').then((r) => r.data),

  getRecentProbes: (nodeId = '', hours = 24, probeType = 'all', points = 200) =>
    http.get<ProbeRecord[]>('/witness/probes', {
      params: { node_id: nodeId, hours, probe_type: probeType, points },
    }).then((r) => r.data),

  getArbitrations: (limit = 20) =>
    http.get<ArbitrationRecord[]>(`/witness/arbitrations?limit=${limit}`).then((r) => r.data),

  // Config & Nodes
  listNodes: () =>
    http.get<NodeRecord[]>('/config/nodes').then((r) => r.data),

  listInterfaces: (nodeId = '') =>
    http.get<InterfaceInfo[]>(`/config/interfaces?node_id=${nodeId}`).then((r) => r.data),

  getConfigFile: (nodeId = '') =>
    http.get<{ content: string }>(`/config/file?node_id=${nodeId}`).then((r) => r.data),

  saveConfigFile: (nodeId = '', data: { content: string; comment?: string }) =>
    http.post(`/config/file?node_id=${nodeId}`, data).then((r) => r.data),

  reloadConfig: (nodeId = '') =>
    http.post(`/config/reload?node_id=${nodeId}`).then((r) => r.data),

  getAuditLogs: (limit = 50, offset = 0) =>
    http.get<{ total: number; items: AuditLog[] }>(`/config/audit-logs?limit=${limit}&offset=${offset}`).then((r) => r.data),
}
