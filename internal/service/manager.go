package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/protocol"
)

type AgentTelemetry struct {
	LastHeartbeat    time.Time
	AgentConnected   bool
	NodeType         string
	WSRttMs          float64
	ClusterID        string
	MemberID         string
	MemberState      string
	Primary          string
	Leader           string
	NetworkHealth    bool
	ServiceAvail     bool
	CoreAvailable    bool
	ActiveSpokes     int
	PeerCount        int
	LocalRole        string
	Term             uint64
	CommitIndex      uint64
	ManifestRevision uint64
	Digest           string
	Witness          protocol.WitnessPayload
}

type NodeManager struct {
	database       *db.DB
	agents         map[string]*executor.AgentExecutor
	telemetry      map[string]AgentTelemetry
	witnessEnabled bool
	// ponytail: last-known views are process-local; persist only if restart recovery is required.
	clusters     map[string]executor.ClusterStatusInfo
	spokes       map[string][]executor.SpokeInfo
	lastPersist  map[string]time.Time
	topologySubs map[chan struct{}]struct{}
	mu           sync.RWMutex
	logHub       *LogHub
}

func NewNodeManager(cfg *config.Config, database *db.DB, logHub *LogHub) *NodeManager {
	mgr := &NodeManager{
		database:     database,
		agents:       make(map[string]*executor.AgentExecutor),
		telemetry:    make(map[string]AgentTelemetry),
		clusters:     make(map[string]executor.ClusterStatusInfo),
		spokes:       make(map[string][]executor.SpokeInfo),
		lastPersist:  make(map[string]time.Time),
		topologySubs: make(map[chan struct{}]struct{}),
		logHub:       logHub,
	}
	if cfg != nil {
		mgr.witnessEnabled = cfg.WitnessEnabled
	}
	return mgr
}

func (m *NodeManager) ValidateMemberDisable(status *executor.ClusterStatusInfo, memberID string) error {
	if !m.witnessEnabled || status == nil {
		return nil
	}
	remaining := make(map[string]bool, 2)
	targetActive := false
	active := 0
	for _, member := range status.Members {
		if member.State != "" && member.State != "active" {
			continue
		}
		active++
		if member.MemberID == memberID {
			targetActive = true
		} else {
			remaining[member.MemberID] = true
		}
	}
	if !targetActive || active != 3 {
		return nil
	}
	if status.Stale || status.ClusterID == "" || status.Leader == "" || status.Digest == "" {
		return fmt.Errorf("拒绝禁用 %s：当前集群视图不完整", memberID)
	}

	ready := make(map[string]bool, 2)
	for _, telemetry := range m.ListAgentTelemetry() {
		if !remaining[telemetry.MemberID] {
			continue
		}
		if telemetry.AgentConnected && time.Since(telemetry.LastHeartbeat) <= witnessStatusFresh &&
			telemetry.ServiceAvail && (telemetry.MemberState == "" || telemetry.MemberState == "active") &&
			telemetry.ClusterID == status.ClusterID && telemetry.Term == status.Term &&
			telemetry.Leader == status.Leader && telemetry.CommitIndex == status.CommitIndex &&
			telemetry.Digest == status.Digest {
			ready[telemetry.MemberID] = true
		}
	}
	if len(ready) != 2 {
		return fmt.Errorf("拒绝禁用 %s：剩余两台 Hub 必须同时在线、服务可用且状态收敛，才能切换到 Manager Witness", memberID)
	}
	return nil
}

func (m *NodeManager) Start(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.syncAgentMemberNames(ctx)
			}
		}
	}()
}

func (m *NodeManager) syncAgentMemberNames(ctx context.Context) {
	m.mu.RLock()
	agentsCopy := make(map[string]*executor.AgentExecutor, len(m.agents))
	for id, exec := range m.agents {
		agentsCopy[id] = exec
	}
	m.mu.RUnlock()

	for nodeID, agentExec := range agentsCopy {
		if agentExec == nil || agentExec.GetNodeType() != "hub" {
			continue
		}
		go func(id string, exec *executor.AgentExecutor) {
			subCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			status, err := exec.GetClusterStatus(subCtx)
			if err == nil && status != nil && status.Member != "" {
				var advIP string
				var prio int
				for _, mb := range status.Members {
					if mb.MemberID == status.Member {
						prio = mb.Priority
						if len(mb.Advertised) > 0 {
							advIP = mb.Advertised[0]
						}
						break
					}
				}
				_, _ = m.database.Exec(
					`UPDATE nodes SET name=?, role=?, term=?, priority=?, advertised_ip=?, updated_at=? WHERE id=?`,
					status.Member, status.LocalRole, status.Term, prio, advIP, time.Now(), id,
				)
			}
		}(nodeID, agentExec)
	}
}

func (m *NodeManager) GetExecutor(nodeID string) (executor.NodeExecutor, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. If specific nodeID requested (not cluster/default), lookup matching agent
	if nodeID != "" && nodeID != "cluster" && nodeID != "default" {
		if agentExec, exists := m.agents[nodeID]; exists && agentExec != nil {
			return agentExec, nil
		}

		// Fallback 1: Lookup in DB by name / alias / advertised IP
		node, err := m.database.GetNodeByNameOrHost(nodeID)
		if err == nil && node != nil {
			if exec, ok := m.agents[node.ID]; ok && exec != nil {
				return exec, nil
			}
		}

		return nil, fmt.Errorf("agent node %s not found or offline", nodeID)
	}

	// 2. Auto-route: when nodeID is empty / "cluster"
	if len(m.agents) == 0 {
		return nil, fmt.Errorf("no opennhrp-agent connected to manager")
	}

	// Prefer agent with leader/primary role in DB
	for id, exec := range m.agents {
		if exec != nil {
			node, err := m.database.GetNode(id)
			if err == nil && node != nil && (node.Role == "leader" || node.Role == "primary") {
				return exec, nil
			}
		}
	}

	// Otherwise return first available online agent
	for _, exec := range m.agents {
		if exec != nil {
			return exec, nil
		}
	}

	return nil, fmt.Errorf("no opennhrp-agent connected to manager")
}

func (m *NodeManager) GetHubExecutor(nodeID string) (executor.NodeExecutor, error) {
	if nodeID != "" && nodeID != "cluster" && nodeID != "default" {
		exec, err := m.GetExecutor(nodeID)
		if err != nil {
			return nil, err
		}
		if exec.GetNodeType() != "hub" {
			return nil, fmt.Errorf("node %s is a spoke, not a Hub", exec.GetNodeID())
		}
		return exec, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	for id, exec := range m.agents {
		if exec == nil || exec.GetNodeType() != "hub" {
			continue
		}
		node, err := m.database.GetNode(id)
		if err == nil && (node.Role == "leader" || node.Role == "primary") {
			return exec, nil
		}
	}
	for _, exec := range m.agents {
		if exec != nil && exec.GetNodeType() == "hub" {
			return exec, nil
		}
	}
	return nil, fmt.Errorf("no Hub opennhrp-agent connected to manager")
}

func (m *NodeManager) RegisterAgent(nodeID, nodeType string, conn *websocket.Conn) *executor.AgentExecutor {
	agentExec := executor.NewAgentExecutor(nodeID, nodeType, conn)
	m.mu.Lock()
	previous := m.agents[nodeID]
	m.agents[nodeID] = agentExec
	m.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}

	// Update DB record
	_, _ = m.database.Exec(
		`INSERT INTO nodes (id, name, type, host, status, updated_at)
		 VALUES (?, ?, ?, ?, 'online', ?)
		 ON CONFLICT(id) DO UPDATE SET type=?, status='online', host=?, updated_at=?`,
		nodeID, nodeID, nodeType, conn.RemoteAddr().String(), time.Now(),
		nodeType, conn.RemoteAddr().String(), time.Now(),
	)
	m.notifyTopology()

	// Fetch cluster status immediately in background to obtain member ID
	if nodeType == "hub" {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			status, err := agentExec.GetClusterStatus(ctx)
			if err == nil && status != nil && status.Member != "" {
				var advIP string
				for _, mb := range status.Members {
					if mb.MemberID == status.Member && len(mb.Advertised) > 0 {
						advIP = mb.Advertised[0]
						break
					}
				}
				_, _ = m.database.Exec(
					`UPDATE nodes SET name=?, role=?, term=?, advertised_ip=?, updated_at=? WHERE id=?`,
					status.Member, status.LocalRole, status.Term, advIP, time.Now(), nodeID,
				)
			}
		}()
	}

	return agentExec
}

func (m *NodeManager) UnregisterAgent(nodeID string, agentExec *executor.AgentExecutor) {
	m.mu.Lock()
	if m.agents[nodeID] != agentExec {
		m.mu.Unlock()
		return
	}
	delete(m.agents, nodeID)
	m.mu.Unlock()

	_, _ = m.database.Exec(
		`UPDATE nodes SET status='offline', updated_at=? WHERE id=?`,
		time.Now(), nodeID,
	)
	m.notifyTopology()
}

func (m *NodeManager) DisconnectAgent(nodeID string) {
	m.mu.RLock()
	agentExec := m.agents[nodeID]
	m.mu.RUnlock()
	if agentExec != nil {
		_ = agentExec.Close()
	}
}

func (m *NodeManager) UpdateHeartbeat(nodeID string, hb protocol.HeartbeatPayload) {
	var rttMs float64
	if !hb.Timestamp.IsZero() {
		dur := time.Since(hb.Timestamp)
		if dur > 0 && dur < 10*time.Second {
			rttMs = float64(dur.Microseconds()) / 1000.0
		}
	}

	m.mu.Lock()
	m.telemetry[nodeID] = AgentTelemetry{
		LastHeartbeat:    time.Now(),
		AgentConnected:   true,
		NodeType:         hb.NodeType,
		WSRttMs:          rttMs,
		ClusterID:        hb.ClusterID,
		MemberID:         hb.MemberID,
		MemberState:      hb.MemberState,
		Primary:          hb.Primary,
		Leader:           hb.Leader,
		NetworkHealth:    hb.NetworkHealth,
		ServiceAvail:     hb.ServiceAvail,
		CoreAvailable:    hb.CoreAvailable,
		ActiveSpokes:     hb.ActiveSpokes,
		PeerCount:        hb.PeerCount,
		LocalRole:        hb.LocalRole,
		Term:             hb.Term,
		CommitIndex:      hb.CommitIndex,
		ManifestRevision: hb.ManifestRevision,
		Digest:           hb.Digest,
		Witness:          hb.Witness,
	}
	persist := time.Since(m.lastPersist[nodeID]) >= 2*time.Second
	if persist {
		m.lastPersist[nodeID] = time.Now()
	}
	m.mu.Unlock()
	if len(hb.ClusterStatus) > 0 {
		var status executor.ClusterStatusInfo
		if json.Unmarshal(hb.ClusterStatus, &status) == nil {
			m.CacheClusterStatus(nodeID, &status)
		}
	}
	if len(hb.Spokes) > 0 {
		var spokes []executor.SpokeInfo
		if json.Unmarshal(hb.Spokes, &spokes) == nil {
			m.CacheSpokes(nodeID, "", spokes)
		}
	}
	if len(hb.Peers) > 0 {
		var peers []executor.SpokeInfo
		if json.Unmarshal(hb.Peers, &peers) == nil {
			m.CacheSpokes(nodeID, "", peers)
		}
	}
	m.notifyTopology()
	if !persist {
		return
	}

	netHealthInt := 0
	if hb.NetworkHealth {
		netHealthInt = 1
	}
	srvAvailInt := 0
	if hb.ServiceAvail {
		srvAvailInt = 1
	}

	if hb.MemberID != "" {
		_, _ = m.database.Exec(
			`UPDATE nodes SET name=?, role=?, term=?, advertised_ip=?, network_health=?, service_avail=?, active_spokes=?, ws_rtt_ms=?, status='online', last_seen=?, updated_at=? WHERE id=?`,
			hb.MemberID, hb.LocalRole, hb.Term, hb.AdvertisedIP, netHealthInt, srvAvailInt, hb.ActiveSpokes, rttMs, time.Now(), time.Now(), nodeID,
		)
	} else {
		_, _ = m.database.Exec(
			`UPDATE nodes SET role=?, term=?, network_health=?, service_avail=?, active_spokes=?, ws_rtt_ms=?, status='online', last_seen=?, updated_at=? WHERE id=?`,
			hb.LocalRole, hb.Term, netHealthInt, srvAvailInt, hb.ActiveSpokes, rttMs, time.Now(), time.Now(), nodeID,
		)
	}
}

func (m *NodeManager) SubscribeTopology() (<-chan struct{}, func()) {
	updates := make(chan struct{}, 1)
	m.mu.Lock()
	m.topologySubs[updates] = struct{}{}
	m.mu.Unlock()
	return updates, func() {
		m.mu.Lock()
		delete(m.topologySubs, updates)
		close(updates)
		m.mu.Unlock()
	}
}

func (m *NodeManager) notifyTopology() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for updates := range m.topologySubs {
		select {
		case updates <- struct{}{}:
		default:
		}
	}
}

func (m *NodeManager) GetNodeTelemetry(nodeID string) (AgentTelemetry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.telemetry[nodeID]
	return t, ok
}

func (m *NodeManager) CacheClusterStatus(nodeID string, status *executor.ClusterStatusInfo) {
	if nodeID == "" || status == nil {
		return
	}
	snapshot := *status
	snapshot.Stale = false
	snapshot.HealthTargets = append([]executor.HealthTargetInfo(nil), status.HealthTargets...)
	snapshot.Members = append([]executor.MemberInfo(nil), status.Members...)
	m.mu.Lock()
	m.clusters[nodeID] = snapshot
	m.mu.Unlock()
}

func (m *NodeManager) GetCachedClusterStatus(nodeID string) (*executor.ClusterStatusInfo, bool) {
	m.mu.RLock()
	snapshot, ok := m.clusters[nodeID]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	snapshot.HealthTargets = append([]executor.HealthTargetInfo(nil), snapshot.HealthTargets...)
	snapshot.Members = append([]executor.MemberInfo(nil), snapshot.Members...)
	return &snapshot, true
}

func (m *NodeManager) CacheSpokes(nodeID, iface string, spokes []executor.SpokeInfo) {
	if nodeID == "" {
		return
	}
	key := nodeID + "\x00" + iface
	snapshot := append([]executor.SpokeInfo{}, spokes...)
	m.mu.Lock()
	metadata := make(map[string]executor.SpokeInfo, len(m.spokes[key]))
	for _, spoke := range m.spokes[key] {
		metadata[spoke.ProtocolAddress] = spoke
	}
	for i := range snapshot {
		snapshot[i].Stale = false
		if previous, ok := metadata[snapshot[i].ProtocolAddress]; ok {
			if snapshot[i].Alias == "" {
				snapshot[i].Alias = previous.Alias
			}
			if snapshot[i].SiteName == "" {
				snapshot[i].SiteName = previous.SiteName
			}
		}
	}
	m.spokes[key] = snapshot
	m.mu.Unlock()
}

func (m *NodeManager) GetCachedSpokes(nodeID, iface string) ([]executor.SpokeInfo, bool) {
	key := nodeID + "\x00" + iface
	m.mu.RLock()
	spokes, ok := m.spokes[key]
	m.mu.RUnlock()
	return append([]executor.SpokeInfo{}, spokes...), ok
}

func (m *NodeManager) ListAgentTelemetry() map[string]AgentTelemetry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]AgentTelemetry, len(m.telemetry))
	for nodeID, telemetry := range m.telemetry {
		_, telemetry.AgentConnected = m.agents[nodeID]
		result[nodeID] = telemetry
	}
	return result
}

func (m *NodeManager) IsAgentHealthy(nodeID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.telemetry[nodeID]
	if !ok {
		return false
	}
	// Check if connected, last heartbeat was within 8 seconds, and service/network are healthy
	_, online := m.agents[nodeID]
	if !online {
		return false
	}
	if time.Since(t.LastHeartbeat) > 8*time.Second {
		return false
	}
	return t.ServiceAvail && t.NetworkHealth
}

func (m *NodeManager) SetTelemetryForTest(nodeID string, tel AgentTelemetry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.telemetry[nodeID] = tel
}

func (m *NodeManager) SetAgentConnectedForTest(nodeID string, connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if connected {
		m.agents[nodeID] = nil
	} else {
		delete(m.agents, nodeID)
	}
}

func (m *NodeManager) ListNodes(_ context.Context) ([]db.NodeRecord, error) {
	rows, err := m.database.Query("SELECT id, name, type, host, status, role, term, priority, advertised_ip, network_health, service_avail, active_spokes, ws_rtt_ms, probe_mode, last_seen, created_at, updated_at FROM nodes ORDER BY priority DESC, id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []db.NodeRecord
	for rows.Next() {
		var n db.NodeRecord
		var netHealthInt, srvAvailInt int
		if err := rows.Scan(&n.ID, &n.Name, &n.Type, &n.Host, &n.Status, &n.Role, &n.Term, &n.Priority, &n.AdvertisedIP, &netHealthInt, &srvAvailInt, &n.ActiveSpokes, &n.WSRttMs, &n.ProbeMode, &n.LastSeen, &n.CreatedAt, &n.UpdatedAt); err != nil {
			return nil, err
		}
		n.NetworkHealth = (netHealthInt == 1)
		n.ServiceAvail = (srvAvailInt == 1)

		if n.Role == "witness" || n.Type == "witness" {
			n.Name = "Witness 见证仲裁中心"
			n.Role = "witness"
			n.Status = "online"
			n.NetworkHealth = true
			n.ServiceAvail = true
		} else {
			if n.Name == "" || strings.HasPrefix(n.Name, "Hub Node ") {
				n.Name = n.ID
			}
			// Check if agent is currently connected in memory
			m.mu.RLock()
			_, online := m.agents[n.ID]
			tel, hasTel := m.telemetry[n.ID]
			m.mu.RUnlock()

			if online {
				n.Status = "online"
			} else {
				n.Status = "offline"
			}
			if hasTel {
				n.NetworkHealth = tel.NetworkHealth
				n.ServiceAvail = tel.ServiceAvail
				n.ActiveSpokes = tel.ActiveSpokes
				if tel.WSRttMs > 0 {
					n.WSRttMs = tel.WSRttMs
				}
			}
		}
		nodes = append(nodes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}
