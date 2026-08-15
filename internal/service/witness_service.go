package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
)

type WitnessService struct {
	cfg             *config.Config
	database        *db.DB
	nodeMgr         *NodeManager
	persistedProbes map[string]persistedProbeState
	clusters        map[string]db.WitnessClusterRecord
	lastAction      map[string]time.Time
	lastAudit       map[string]string
	termFloor       map[string]uint64
	startedAt       time.Time
	command         func(context.Context, string, string) error
	mu              sync.Mutex
}

const (
	probePersistInterval   = 30 * time.Second
	probeRetentionHours    = 24
	witnessStatusFresh     = 1500 * time.Millisecond
	witnessCommandMin      = 500 * time.Millisecond
	witnessCommandMax      = 1500 * time.Millisecond
	witnessCommandSlack    = 250 * time.Millisecond
	witnessRenewInterval   = time.Second
	witnessLeaseTTL        = 3 * time.Second
	witnessRestartSilence  = 3500 * time.Millisecond
	witnessRestartTakeover = 10 * time.Second
)

type persistedProbeState struct {
	recordedAt time.Time
	targetIP   string
	source     string
	detail     string
	lossRate   float64
	success    bool
}

type NodeSLASummary struct {
	NodeID            string  `json:"node_id"`
	AvgRttMs          float64 `json:"avg_rtt_ms"`
	LossRate          float64 `json:"loss_rate"`
	L3Healthy         bool    `json:"l3_healthy"`
	L4Healthy         bool    `json:"l4_healthy"`
	AgentHealthy      bool    `json:"agent_healthy"`
	DataHealthy       bool    `json:"data_healthy"`
	FirewallProtected bool    `json:"firewall_protected"`
	LatencySource     string  `json:"latency_source"` // icmp, ws
	ActiveSpokes      int     `json:"active_spokes"`
	OverallState      string  `json:"overall_state"` // healthy, degraded, critical
	LastChecked       string  `json:"last_checked"`
}

func NewWitnessService(cfg *config.Config, database *db.DB, nodeMgr *NodeManager) *WitnessService {
	return &WitnessService{
		cfg:             cfg,
		database:        database,
		nodeMgr:         nodeMgr,
		persistedProbes: make(map[string]persistedProbeState),
		clusters:        make(map[string]db.WitnessClusterRecord),
		lastAction:      make(map[string]time.Time),
		lastAudit:       make(map[string]string),
		termFloor:       make(map[string]uint64),
	}
}

func (w *WitnessService) Start(ctx context.Context) {
	if !w.cfg.WitnessEnabled {
		log.Println("[Witness] Witness engine disabled by config")
		return
	}

	log.Printf("[Witness] Witness lease coordinator started (renew: 1s, lease: 3s)")
	w.startedAt = time.Now()
	if w.database != nil {
		if records, err := w.database.GetWitnessClusters(); err == nil {
			w.mu.Lock()
			for _, record := range records {
				w.clusters[record.ClusterID] = record
			}
			w.mu.Unlock()
		}
	}

	ticker := time.NewTicker(time.Duration(w.cfg.WitnessInterval) * time.Second)
	defer ticker.Stop()
	quorumTicker := time.NewTicker(250 * time.Millisecond)
	go func() {
		defer quorumTicker.Stop()
		w.runQuorumLoop(ctx, quorumTicker.C)
	}()

	cleanupTicker := time.NewTicker(1 * time.Hour)
	defer cleanupTicker.Stop()

	// Initial cleanup keeps exactly the history range exposed by the UI.
	if w.database != nil {
		_ = w.database.CleanupOldProbes(probeRetentionHours)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-cleanupTicker.C:
			if w.database != nil {
				_ = w.database.CleanupOldProbes(probeRetentionHours)
			}
		case <-ticker.C:
			w.runProbeCycle(ctx)
		}
	}
}

func (w *WitnessService) runQuorumLoop(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticks:
			w.runQuorumCycle(ctx, now)
		}
	}
}

func (w *WitnessService) runProbeCycle(ctx context.Context) {
	nodes, err := w.nodeMgr.ListNodes(ctx)
	if err != nil {
		return
	}

	for _, n := range nodes {
		if n.Role == "witness" || n.Type == "spoke" {
			continue
		}

		telemetry, hasTel := w.nodeMgr.GetNodeTelemetry(n.ID)
		agentHealthy := w.nodeMgr.IsAgentHealthy(n.ID)

		if n.ProbeMode == "agent_only" {
			wsRtt := 0.0
			if hasTel {
				wsRtt = telemetry.WSRttMs
			}
			_ = w.saveProbe(db.WitnessProbeRecord{
				TargetNodeID: n.ID,
				ProbeType:    "agent_telemetry",
				TargetIP:     n.Host,
				RttMs:        wsRtt,
				LossRate:     0.0,
				Success:      agentHealthy,
				Detail:       fmt.Sprintf("Agent Telemetry: network_health=%v, service_avail=%v", n.NetworkHealth, n.ServiceAvail),
			})
			continue
		}

		// 1. Probe L4 Port 49002 if host is available
		host := w.probeHost(ctx, n)

		// L4 HA Port 49002 probe
		l4Addr := net.JoinHostPort(host, "49002")
		start := time.Now()
		conn, err := net.DialTimeout("tcp", l4Addr, 1500*time.Millisecond)
		rttL4 := float64(time.Since(start).Microseconds()) / 1000.0
		l4Ok := err == nil
		if conn != nil {
			conn.Close()
		}

		// When agent is healthy but L4 fails: firewall is blocking inbound, mark as success with WS RTT
		var lossL4 float64
		if !l4Ok {
			if agentHealthy {
				// Firewall protected: use WS RTT, no loss
				wsRtt := 0.0
				if hasTel {
					wsRtt = telemetry.WSRttMs
				}
				l4Ok = true
				rttL4 = wsRtt
				lossL4 = 0.0
			} else {
				lossL4 = 1.0
				rttL4 = 0.0
			}
		}

		detailL4 := fmt.Sprintf("TCP 49002 probe: %v", err)
		if !l4Ok && agentHealthy {
			detailL4 = "TCP 49002 inbound blocked by firewall (Agent telemetry healthy)"
		} else if l4Ok && err != nil && agentHealthy {
			detailL4 = fmt.Sprintf("TCP 49002 blocked by firewall, Agent WS RTT %.1fms", rttL4)
		}

		_ = w.saveProbe(db.WitnessProbeRecord{
			TargetNodeID: n.ID,
			ProbeType:    "l4_port",
			TargetIP:     l4Addr,
			RttMs:        rttL4,
			LossRate:     lossL4,
			Success:      l4Ok,
			Detail:       detailL4,
		})

		// 2. L3 ICMP / System Ping Probe
		rttL3, loss, l3Ok := w.pingTarget(host)
		// When agent is healthy but L3 ping fails: firewall blocks ICMP, mark as success with WS RTT
		if !l3Ok && agentHealthy {
			wsRtt := 0.0
			if hasTel {
				wsRtt = telemetry.WSRttMs
			}
			l3Ok = true
			rttL3 = wsRtt
			loss = 0.0
		}

		detailL3 := fmt.Sprintf("ICMP ping: loss %.1f%%", loss*100)
		if l3Ok && rttL3 > 0 && loss == 0 && agentHealthy && rttL3 == telemetry.WSRttMs {
			detailL3 = fmt.Sprintf("ICMP blocked by firewall, Agent WS RTT %.1fms", rttL3)
		}

		_ = w.saveProbe(db.WitnessProbeRecord{
			TargetNodeID: n.ID,
			ProbeType:    "l3_nbma",
			TargetIP:     host,
			RttMs:        rttL3,
			LossRate:     loss,
			Success:      l3Ok,
			Detail:       detailL3,
		})
	}

}

func (w *WitnessService) saveProbe(probe db.WitnessProbeRecord) error {
	if w.database == nil {
		return fmt.Errorf("witness database is unavailable")
	}

	if probe.RecordedAt.IsZero() {
		probe.RecordedAt = time.Now()
	}
	key := probe.TargetNodeID + "\x00" + probe.ProbeType
	source := probeSource(probe)
	previous, exists := w.persistedProbes[key]
	if exists && probe.RecordedAt.Sub(previous.recordedAt) < probePersistInterval &&
		probe.TargetIP == previous.targetIP && source == previous.source &&
		(probe.ProbeType != "agent_telemetry" || probe.Detail == previous.detail) &&
		probe.LossRate == previous.lossRate && probe.Success == previous.success {
		return nil
	}

	if err := w.database.SaveWitnessProbe(probe); err != nil {
		return err
	}
	w.persistedProbes[key] = persistedProbeState{
		recordedAt: probe.RecordedAt,
		targetIP:   probe.TargetIP,
		source:     source,
		detail:     probe.Detail,
		lossRate:   probe.LossRate,
		success:    probe.Success,
	}
	return nil
}

func probeSource(probe db.WitnessProbeRecord) string {
	if probe.ProbeType == "agent_telemetry" {
		return "agent"
	}
	if strings.Contains(strings.ToLower(probe.Detail), "firewall") {
		return "firewall"
	}
	return "direct"
}

func (w *WitnessService) probeHost(ctx context.Context, node db.NodeRecord) string {
	host := node.Host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	exec, err := w.nodeMgr.GetHubExecutor(node.ID)
	if err != nil {
		return host
	}
	status, err := exec.GetClusterStatus(ctx)
	if err != nil {
		return host
	}
	for _, member := range status.Members {
		if member.MemberID != status.Member {
			continue
		}
		for _, address := range member.Advertised {
			ip := net.ParseIP(address)
			if ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() {
				return address
			}
		}
	}
	return host
}

func (w *WitnessService) pingTarget(host string) (rttMs float64, lossRate float64, success bool) {
	out, err := exec.Command("ping", "-c", "2", "-W", "1", host).CombinedOutput()
	if err != nil {
		return 0, 1.0, false
	}
	output := string(out)
	// Parse loss and avg rtt
	if strings.Contains(output, "0% packet loss") {
		lossRate = 0.0
		success = true
	} else if strings.Contains(output, "100% packet loss") {
		return 0, 1.0, false
	} else {
		lossRate = 0.5
		success = true
	}

	// Find avg rtt e.g. "min/avg/max/mdev = 0.040/0.055/0.070/0.015 ms"
	if idx := strings.Index(output, "min/avg/max"); idx != -1 {
		line := output[idx:]
		if parts := strings.Split(line, "="); len(parts) >= 2 {
			stats := strings.Split(strings.TrimSpace(parts[1]), "/")
			if len(stats) >= 2 {
				rttMs, _ = strconv.ParseFloat(stats[1], 64)
			}
		}
	}
	return rttMs, lossRate, success
}

type witnessHub struct {
	NodeID string
	AgentTelemetry
}

type WitnessQuorumMember struct {
	NodeID          string `json:"node_id"`
	MemberID        string `json:"member_id"`
	AgentConnected  bool   `json:"agent_connected"`
	Fresh           bool   `json:"fresh"`
	Role            string `json:"role"`
	Term            uint64 `json:"term"`
	CommitIndex     uint64 `json:"commit_index"`
	Digest          string `json:"digest"`
	PeerVote        bool   `json:"peer_vote"`
	ManagerVote     bool   `json:"manager_vote"`
	QuorumAvailable bool   `json:"quorum_available"`
	Fenced          bool   `json:"fenced"`
}

type WitnessQuorumStatus struct {
	ClusterID           string                `json:"cluster_id"`
	Mode                string                `json:"mode"`
	Policy              string                `json:"policy"`
	Epoch               string                `json:"epoch"`
	Voters              int                   `json:"voters"`
	Required            int                   `json:"required"`
	Votes               int                   `json:"votes"`
	Leader              string                `json:"leader"`
	Term                uint64                `json:"term"`
	Holder              string                `json:"holder"`
	LeaseRemainingMS    uint64                `json:"lease_remaining_ms"`
	FallbackRemainingMS uint64                `json:"fallback_remaining_ms"`
	Transition          string                `json:"transition"`
	DecisionReason      string                `json:"decision_reason"`
	Members             []WitnessQuorumMember `json:"members"`
}

func newWitnessEpoch() (string, error) {
	var epoch [16]byte
	if _, err := rand.Read(epoch[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(epoch[:]), nil
}

func (w *WitnessService) saveCluster(record db.WitnessClusterRecord) bool {
	record.UpdatedAt = time.Now()
	if w.database == nil || w.database.SaveWitnessCluster(record) != nil {
		return false
	}
	w.mu.Lock()
	w.clusters[record.ClusterID] = record
	w.mu.Unlock()
	return true
}

func (w *WitnessService) auditDecision(record db.WitnessClusterRecord,
	hubs []witnessHub, decision, reason string) {
	key := fmt.Sprintf("%s|%s|%d|%s", decision, record.Holder, record.Term,
		record.Transition)
	w.mu.Lock()
	if w.lastAudit[record.ClusterID] == key {
		w.mu.Unlock()
		return
	}
	w.lastAudit[record.ClusterID] = key
	w.mu.Unlock()
	if w.database == nil {
		return
	}
	primaryNodeID, backupNodeID := "", ""
	involvedNodeIDs := make([]string, 0, len(hubs))
	seenNodeIDs := make(map[string]bool, len(hubs))
	for _, hub := range hubs {
		if hub.NodeID != "" && !seenNodeIDs[hub.NodeID] {
			seenNodeIDs[hub.NodeID] = true
			involvedNodeIDs = append(involvedNodeIDs, hub.NodeID)
		}
		if hub.MemberID == hub.Primary {
			primaryNodeID = hub.NodeID
		} else if backupNodeID == "" {
			backupNodeID = hub.NodeID
		}
	}
	sort.Slice(involvedNodeIDs, func(i, j int) bool {
		if involvedNodeIDs[i] == primaryNodeID {
			return true
		}
		if involvedNodeIDs[j] == primaryNodeID {
			return false
		}
		return involvedNodeIDs[i] < involvedNodeIDs[j]
	})
	_ = w.database.SaveArbitration(db.WitnessArbitrationRecord{
		Term:            record.Term,
		PrimaryNodeID:   primaryNodeID,
		BackupNodeID:    backupNodeID,
		InvolvedNodeIDs: involvedNodeIDs,
		Decision:        decision,
		Reason:          reason,
	})
}

func (w *WitnessService) sendWitnessCommand(ctx context.Context, nodeID,
	command string) error {
	if w.command != nil {
		return w.command(ctx, nodeID, command)
	}
	exec, err := w.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		return err
	}
	return exec.SendWitnessCommand(ctx, command)
}

func (w *WitnessService) commandHubs(ctx context.Context, hubs []witnessHub,
	command string) map[string]error {
	limit := witnessCommandTimeout(hubs)
	errorsByNode := make(map[string]error, len(hubs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, hub := range hubs {
		hub := hub
		wg.Add(1)
		go func() {
			defer wg.Done()
			commandCtx, cancel := context.WithTimeout(ctx, limit)
			defer cancel()
			err := w.sendWitnessCommand(commandCtx, hub.NodeID, command)
			mu.Lock()
			errorsByNode[hub.NodeID] = err
			mu.Unlock()
		}()
	}
	wg.Wait()
	return errorsByNode
}

func witnessCommandTimeout(hubs []witnessHub) time.Duration {
	limit := witnessCommandMin
	for _, hub := range hubs {
		candidate := time.Duration(hub.WSRttMs*2)*time.Millisecond + witnessCommandSlack
		if candidate > limit {
			limit = candidate
		}
	}
	if limit > witnessCommandMax {
		return witnessCommandMax
	}
	return limit
}

func allCommandsSucceeded(results map[string]error) bool {
	if len(results) == 0 {
		return false
	}
	for _, err := range results {
		if err != nil {
			return false
		}
	}
	return true
}

func commandFailure(results map[string]error) string {
	nodes := make([]string, 0, len(results))
	for nodeID, err := range results {
		if err != nil {
			nodes = append(nodes, fmt.Sprintf("%s: %v", nodeID, err))
		}
	}
	sort.Strings(nodes)
	return strings.Join(nodes, "; ")
}

func freshClusterHubs(hubs []witnessHub, now time.Time) []witnessHub {
	result := make([]witnessHub, 0, len(hubs))
	seen := make(map[string]bool)
	for _, hub := range hubs {
		if !hub.AgentConnected || hub.MemberID == "" ||
			(hub.MemberState != "" && hub.MemberState != "active") ||
			now.Sub(hub.LastHeartbeat) > witnessStatusFresh {
			continue
		}
		if seen[hub.MemberID] {
			return nil
		}
		seen[hub.MemberID] = true
		result = append(result, hub)
	}
	return result
}

func freshWitnessHubs(hubs []witnessHub, now time.Time) []witnessHub {
	fresh := freshClusterHubs(hubs, now)
	if fresh == nil {
		return nil
	}
	result := make([]witnessHub, 0, len(fresh))
	for _, hub := range fresh {
		if hub.Witness.Capable {
			result = append(result, hub)
		}
	}
	return result
}

func (w *WitnessService) actionReady(clusterID string, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if now.Sub(w.lastAction[clusterID]) < witnessCommandMin {
		return false
	}
	w.lastAction[clusterID] = now
	return true
}

func (w *WitnessService) termViewsConverged(clusterID string, hubs []witnessHub) bool {
	w.mu.Lock()
	floor := w.termFloor[clusterID]
	w.mu.Unlock()
	if floor == 0 {
		return true
	}
	if len(hubs) != 2 || hubs[0].Leader == "" || hubs[0].Leader != hubs[1].Leader ||
		hubs[0].Term < floor || hubs[1].Term < floor {
		return false
	}
	w.mu.Lock()
	if w.termFloor[clusterID] == floor {
		delete(w.termFloor, clusterID)
	}
	w.mu.Unlock()
	return true
}

func witnessLeaseFailure(err error) (string, uint64) {
	if errors.Is(err, context.DeadlineExceeded) {
		return "witness_lease_command_timeout", 0
	}
	var commandErr *executor.AgentCommandError
	if !errors.As(err, &commandErr) {
		return "witness_lease_command_failed", 0
	}
	for _, line := range strings.Split(commandErr.Detail, "\n") {
		value, found := strings.CutPrefix(strings.TrimSpace(line), "Current-Term:")
		if !found {
			continue
		}
		term, parseErr := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if parseErr == nil {
			return "witness_lease_term_conflict", term
		}
	}
	return "witness_lease_rejected", 0
}

func (w *WitnessService) leaseReady(clusterID string, now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if now.Sub(w.lastAction[clusterID]) < witnessRenewInterval {
		return false
	}
	w.lastAction[clusterID] = now
	return true
}

func (w *WitnessService) activateCluster(ctx context.Context,
	record db.WitnessClusterRecord, hubs []witnessHub) (db.WitnessClusterRecord, bool) {
	prepare := w.commandHubs(ctx, hubs,
		fmt.Sprintf("ha witness prepare epoch %s", record.Epoch))
	if !allCommandsSucceeded(prepare) {
		record.Mode = "preparing"
		record.Transition = "prepare_partial"
		record.Reason = commandFailure(prepare)
		w.saveCluster(record)
		w.auditDecision(record, hubs, "witness_prepare_rejected", record.Reason)
		return record, false
	}
	activate := w.commandHubs(ctx, hubs,
		fmt.Sprintf("ha witness activate epoch %s", record.Epoch))
	if !allCommandsSucceeded(activate) {
		record.Mode = "preparing"
		record.Transition = "activate_partial"
		record.Reason = commandFailure(activate)
		w.saveCluster(record)
		w.auditDecision(record, hubs, "witness_activate_rejected", record.Reason)
		return record, false
	}
	record.Mode = "active"
	record.Transition = "active"
	record.Reason = "both Hubs acknowledged prepare and activate"
	if !w.saveCluster(record) {
		return record, false
	}
	w.auditDecision(record, hubs, "witness_activated", record.Reason)
	return record, true
}

func selectWitnessHolder(hubs []witnessHub, record db.WitnessClusterRecord,
	now time.Time) (witnessHub, uint64, string, bool) {
	eligible := make([]witnessHub, 0, len(hubs))
	for _, hub := range hubs {
		if hub.ServiceAvail {
			eligible = append(eligible, hub)
		}
	}
	if len(eligible) == 0 {
		return witnessHub{}, 0, "no fresh serviceable Hub", false
	}
	for _, hub := range eligible {
		if hub.MemberID == record.Holder {
			term := record.Term
			if hub.Term > term {
				term = hub.Term
			}
			if hub.Leader == hub.MemberID {
				return hub, term, "healthy current holder retained", true
			}
		}
	}
	if record.Holder != "" && now.Before(record.SafeUntil) {
		return witnessHub{}, 0, "waiting for old holder lease safety deadline", false
	}
	for i := 0; i < len(eligible); i++ {
		for j := i + 1; j < len(eligible); j++ {
			if eligible[i].Term == eligible[j].Term &&
				eligible[i].CommitIndex == eligible[j].CommitIndex &&
				eligible[i].Digest != eligible[j].Digest {
				return witnessHub{}, 0, "equal term/index with conflicting digest", false
			}
		}
	}
	if len(eligible) > 1 {
		agreedLeader := eligible[0].Leader
		term := uint64(0)
		var agreedHolder witnessHub
		for _, hub := range eligible {
			if agreedLeader == "" || hub.Leader != agreedLeader {
				agreedLeader = ""
				break
			}
			if hub.Term > term {
				term = hub.Term
			}
			if hub.MemberID == agreedLeader {
				agreedHolder = hub
			}
		}
		if agreedLeader != "" && agreedHolder.MemberID != "" {
			return agreedHolder, term, "fresh Hubs agree on current Leader", true
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if eligible[i].Term != eligible[j].Term {
			return eligible[i].Term > eligible[j].Term
		}
		if eligible[i].CommitIndex != eligible[j].CommitIndex {
			return eligible[i].CommitIndex > eligible[j].CommitIndex
		}
		iPrimary := eligible[i].MemberID == eligible[i].Primary
		jPrimary := eligible[j].MemberID == eligible[j].Primary
		if iPrimary != jPrimary {
			return iPrimary
		}
		return eligible[i].MemberID < eligible[j].MemberID
	})
	holder := eligible[0]
	term := holder.Term
	for _, hub := range eligible {
		if hub.Term > term {
			term = hub.Term
		}
	}
	for _, hub := range eligible {
		if hub.Term != term || hub.Leader != holder.MemberID {
			term++
			break
		}
	}
	return holder, term, "selected by term, commit index, then fixed Primary", true
}

func (w *WitnessService) grantLease(ctx context.Context,
	record db.WitnessClusterRecord, hubs []witnessHub, now time.Time) {
	holder, term, reason, ok := selectWitnessHolder(hubs, record, now)
	if !ok {
		record.Transition = "lease_refused"
		record.Reason = reason
		w.saveCluster(record)
		w.auditDecision(record, hubs, "witness_lease_refused", reason)
		return
	}
	previousHolder, previousTerm := record.Holder, record.Term
	record.Sequence++
	if !w.saveCluster(record) {
		return
	}
	issuedAt := time.Now()
	command := fmt.Sprintf(
		"ha witness lease epoch %s term %d holder %s sequence %d ttl-ms 3000",
		record.Epoch, term, holder.MemberID, record.Sequence)
	results := w.commandHubs(ctx, hubs, command)
	safetyDeadline := issuedAt.Add(witnessCommandTimeout(hubs) + witnessLeaseTTL)
	if results[holder.NodeID] != nil {
		if safetyDeadline.After(record.SafeUntil) {
			record.SafeUntil = safetyDeadline
		}
		record.Transition = "lease_uncertain"
		record.Reason = fmt.Sprintf("holder command failed: %v", results[holder.NodeID])
		w.saveCluster(record)
		decision, currentTerm := witnessLeaseFailure(results[holder.NodeID])
		if currentTerm > term {
			w.mu.Lock()
			w.termFloor[record.ClusterID] = currentTerm
			w.mu.Unlock()
		}
		w.auditDecision(record, []witnessHub{holder}, decision, record.Reason)
		return
	}
	record.Holder = holder.MemberID
	record.Term = term
	record.SafeUntil = safetyDeadline
	record.Transition = "active"
	record.Reason = reason
	w.saveCluster(record)
	if previousHolder != record.Holder || previousTerm != record.Term {
		w.auditDecision(record, []witnessHub{holder}, "witness_lease_granted", reason)
	}
}

func (w *WitnessService) runQuorumCycle(ctx context.Context, now time.Time) {
	if w.nodeMgr == nil || now.Sub(w.startedAt) < witnessRestartSilence {
		return
	}
	telemetry := w.nodeMgr.ListAgentTelemetry()
	groups := make(map[string][]witnessHub)
	for nodeID, status := range telemetry {
		if status.ClusterID != "" {
			groups[status.ClusterID] = append(groups[status.ClusterID],
				witnessHub{NodeID: nodeID, AgentTelemetry: status})
		}
	}
	for clusterID, observed := range groups {
		clusterHubs := freshClusterHubs(observed, now)
		fresh := freshWitnessHubs(observed, now)
		w.mu.Lock()
		record, exists := w.clusters[clusterID]
		w.mu.Unlock()
		hubMajority := false
		for _, hub := range clusterHubs {
			hubMajority = hubMajority || hub.Witness.Policy == "hub-majority"
		}
		if len(clusterHubs) == 1 && clusterHubs[0].Witness.Policy == "legacy" {
			if exists && record.Mode != "legacy" {
				record.Mode = "legacy"
				record.Holder = ""
				record.Term = 0
				record.SafeUntil = time.Time{}
				record.Transition = "single_hub_legacy"
				record.Reason = "the sole active Hub uses legacy availability-first"
				w.saveCluster(record)
				w.auditDecision(record, clusterHubs, "witness_disabled", record.Reason)
			}
			continue
		}
		if hubMajority {
			if exists && record.Mode != "legacy" {
				record.Mode = "legacy"
				record.Holder = ""
				record.Term = 0
				record.SafeUntil = time.Time{}
				record.Transition = "hub_majority"
				record.Reason = "three or more online Hubs use Hub majority"
				w.saveCluster(record)
				w.auditDecision(record, clusterHubs, "witness_disabled", record.Reason)
			}
			continue
		}
		if exists && len(fresh) == 2 &&
			fresh[0].LastHeartbeat.After(record.UpdatedAt) &&
			fresh[1].LastHeartbeat.After(record.UpdatedAt) &&
			fresh[0].Witness.Mode == "legacy" && fresh[0].Witness.Epoch == record.Epoch &&
			fresh[1].Witness.Mode == "legacy" && fresh[1].Witness.Epoch == record.Epoch &&
			record.Mode != "legacy" {
			record.Mode = "legacy"
			record.Holder = ""
			record.Term = 0
			record.SafeUntil = time.Time{}
			record.Transition = "legacy_fallback_observed"
			record.Reason = "both Hubs completed coordinated legacy fallback"
			w.saveCluster(record)
			w.auditDecision(record, fresh, "witness_disabled", record.Reason)
			continue
		}
		if !exists && len(fresh) == 2 && fresh[0].Witness.Epoch != "" &&
			fresh[0].Witness.Epoch == fresh[1].Witness.Epoch &&
			fresh[0].Witness.Mode == fresh[1].Witness.Mode &&
			fresh[0].Witness.Mode != "legacy" {
			record = db.WitnessClusterRecord{
				ClusterID: clusterID, Epoch: fresh[0].Witness.Epoch,
				Mode: fresh[0].Witness.Mode, Holder: fresh[0].Witness.LeaseHolder,
				Term:       fresh[0].Witness.LeaseTerm,
				Sequence:   fresh[0].Witness.LeaseSequence,
				Transition: "recovered", Reason: "recovered state from both Hubs",
			}
			if fresh[1].Witness.LeaseSequence > record.Sequence {
				record.Sequence = fresh[1].Witness.LeaseSequence
			}
			if record.Mode == "disabling" {
				record.Mode = "active"
				record.Transition = "recovered_disabling"
			}
			exists = w.saveCluster(record)
		}
		if !exists || record.Mode == "legacy" {
			if len(fresh) != 2 || !w.actionReady(clusterID, now) {
				continue
			}
			epoch, err := newWitnessEpoch()
			if err != nil {
				continue
			}
			record = db.WitnessClusterRecord{
				ClusterID: clusterID, Epoch: epoch, Mode: "preparing",
				Transition: "preparing", Reason: "two compatible Hubs detected",
			}
			if !w.saveCluster(record) {
				continue
			}
			var activated bool
			record, activated = w.activateCluster(ctx, record, fresh)
			if !activated {
				continue
			}
			continue
		}
		if record.Mode == "preparing" {
			if len(fresh) != 2 || !w.actionReady(clusterID, now) {
				continue
			}
			var activated bool
			record, activated = w.activateCluster(ctx, record, fresh)
			if !activated {
				continue
			}
			continue
		}
		if record.Mode != "active" || len(fresh) == 0 {
			continue
		}
		// A standby Agent can reconnect before the current holder after a Manager
		// restart. Give the holder time to return before issuing a takeover lease.
		if len(fresh) == 1 && now.Sub(w.startedAt) < witnessRestartTakeover &&
			(record.Holder == "" || fresh[0].MemberID != record.Holder ||
				fresh[0].Leader != record.Holder) {
			continue
		}
		epochConflict := false
		for _, hub := range fresh {
			epochConflict = epochConflict || hub.Witness.Epoch != record.Epoch
		}
		if epochConflict {
			if len(fresh) != 2 || !w.actionReady(clusterID, now) {
				continue
			}
			record.Mode = "preparing"
			record.Transition = "epoch_reprepare"
			record.Reason = "reconciling Hub Witness epochs"
			if !w.saveCluster(record) {
				continue
			}
			w.activateCluster(ctx, record, fresh)
			continue
		}
		resume := false
		for _, hub := range fresh {
			if hub.Witness.Epoch == record.Epoch &&
				(hub.Witness.Mode == "disabling" || hub.Witness.Mode == "preparing") {
				resume = true
			}
		}
		if resume {
			if !w.actionReady(clusterID, now) {
				continue
			}
			results := w.commandHubs(ctx, fresh,
				fmt.Sprintf("ha witness activate epoch %s", record.Epoch))
			if !allCommandsSucceeded(results) {
				record.Transition = "activate_partial"
				record.Reason = commandFailure(results)
				w.saveCluster(record)
				w.auditDecision(record, fresh, "witness_activate_rejected", record.Reason)
				continue
			}
		}
		stateConflict := false
		for _, hub := range fresh {
			if hub.Witness.Mode != "active" || hub.Witness.Epoch != record.Epoch {
				if !(resume && hub.Witness.Epoch == record.Epoch) {
					stateConflict = true
				}
			}
		}
		if stateConflict {
			record.Transition = "state_conflict"
			record.Reason = "Hub Witness mode or epoch does not match Manager state"
			w.saveCluster(record)
			w.auditDecision(record, fresh, "witness_state_conflict", record.Reason)
			continue
		}
		if !w.termViewsConverged(clusterID, fresh) {
			continue
		}
		if !w.leaseReady(clusterID, now) {
			continue
		}
		w.grantLease(ctx, record, fresh, now)
	}
}

func (w *WitnessService) GetQuorumStatus() WitnessQuorumStatus {
	status := WitnessQuorumStatus{Mode: "legacy", Policy: "legacy",
		Transition: "legacy", DecisionReason: "Witness has not been activated"}
	if w.nodeMgr == nil {
		return status
	}
	telemetry := w.nodeMgr.ListAgentTelemetry()
	w.mu.Lock()
	clusterIDs := make([]string, 0, len(w.clusters))
	for clusterID := range w.clusters {
		clusterIDs = append(clusterIDs, clusterID)
	}
	sort.Strings(clusterIDs)
	hasRecord := len(clusterIDs) > 0
	if hasRecord {
		record := w.clusters[clusterIDs[0]]
		status.ClusterID = record.ClusterID
		status.Mode = record.Mode
		status.Epoch = record.Epoch
		status.Holder = record.Holder
		status.Term = record.Term
		status.Transition = record.Transition
		status.DecisionReason = record.Reason
	}
	w.mu.Unlock()
	now := time.Now()
	for nodeID, hub := range telemetry {
		if status.ClusterID == "" {
			status.ClusterID = hub.ClusterID
		}
		if hub.ClusterID != status.ClusterID {
			continue
		}
		if !hasRecord {
			status.Mode = hub.Witness.Mode
			status.Epoch = hub.Witness.Epoch
			status.Holder = hub.Witness.LeaseHolder
			status.Term = hub.Witness.LeaseTerm
			status.Transition = hub.Witness.Mode
			status.DecisionReason = "reported directly by Hub Agent"
		}
		if hub.MemberID == hub.Leader || status.Voters == 0 {
			status.Policy = hub.Witness.Policy
			status.Voters = hub.Witness.Voters
			status.Required = hub.Witness.Required
			status.Votes = hub.Witness.Votes
		}
		fresh := hub.AgentConnected && now.Sub(hub.LastHeartbeat) <= witnessStatusFresh
		fenced := hub.LocalRole == "isolated" ||
			(hub.MemberID == hub.Leader && !hub.Witness.QuorumAvailable)
		status.Members = append(status.Members, WitnessQuorumMember{
			NodeID: nodeID, MemberID: hub.MemberID, AgentConnected: hub.AgentConnected,
			Fresh: fresh, Role: hub.LocalRole, Term: hub.Term,
			CommitIndex: hub.CommitIndex, Digest: hub.Digest,
			PeerVote: hub.Witness.PeerVote, ManagerVote: hub.Witness.ManagerVote,
			QuorumAvailable: hub.Witness.QuorumAvailable, Fenced: fenced,
		})
		if hub.Leader != "" {
			status.Leader = hub.Leader
		}
		if hub.Witness.LeaseHolder == status.Holder &&
			hub.Witness.LeaseRemainingMS > status.LeaseRemainingMS {
			status.LeaseRemainingMS = hub.Witness.LeaseRemainingMS
		}
		if hub.Witness.FallbackRemainingMS > status.FallbackRemainingMS {
			status.FallbackRemainingMS = hub.Witness.FallbackRemainingMS
		}
	}
	sort.Slice(status.Members, func(i, j int) bool {
		return status.Members[i].MemberID < status.Members[j].MemberID
	})
	return status
}

func (w *WitnessService) IsNodeHealthy(nodeID string, probes []db.WitnessProbeRecord) bool {
	if IsNodeProbeHealthy(probes) {
		return true
	}
	if w.nodeMgr != nil && w.nodeMgr.IsAgentHealthy(nodeID) {
		return true
	}
	return false
}

func IsNodeProbeHealthy(probes []db.WitnessProbeRecord) bool {
	var l3, l4 bool
	var haveL3, haveL4 bool
	for _, p := range probes {
		switch p.ProbeType {
		case "l3_nbma":
			if !haveL3 {
				l3, haveL3 = p.Success, true
			}
		case "l4_port":
			if !haveL4 {
				l4, haveL4 = p.Success, true
			}
		case "agent_telemetry":
			if !haveL3 {
				l3, haveL3 = p.Success, true
			}
			if !haveL4 {
				l4, haveL4 = p.Success, true
			}
		}
	}
	return haveL3 && haveL4 && l3 && l4
}

func isNodeProbeHealthy(probes []db.WitnessProbeRecord) bool {
	return IsNodeProbeHealthy(probes)
}

func (w *WitnessService) GetSLAMatrix(ctx context.Context) ([]NodeSLASummary, error) {
	nodes, err := w.nodeMgr.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	var matrix []NodeSLASummary
	for _, n := range nodes {
		if n.Role == "witness" || n.Type == "spoke" {
			continue
		}
		probes, _ := w.database.GetRecentProbes(n.ID, 10)
		telemetry, hasTel := w.nodeMgr.GetNodeTelemetry(n.ID)
		agentHealthy := w.nodeMgr.IsAgentHealthy(n.ID)

		summary := SummarizeNodeSLA(n.ID, probes, n, telemetry, hasTel, agentHealthy)
		matrix = append(matrix, summary)
	}

	return matrix, nil
}

func SummarizeNodeSLA(nodeID string, probes []db.WitnessProbeRecord, node db.NodeRecord, tel AgentTelemetry, hasTel bool, agentHealthy bool) NodeSLASummary {
	summary := NodeSLASummary{
		NodeID:        nodeID,
		OverallState:  "unknown",
		LatencySource: "icmp",
		AgentHealthy:  agentHealthy,
	}

	if hasTel && agentHealthy {
		summary.ActiveSpokes = tel.ActiveSpokes
	}

	var l3TotalRtt float64
	var l3SuccessCount int
	var l3LossCount int
	var l3TotalProbes int
	var haveL3, haveL4 bool

	for _, probe := range probes {
		switch probe.ProbeType {
		case "l3_nbma":
			l3TotalProbes++
			if !probe.Success || probe.LossRate >= 1.0 {
				l3LossCount++
			} else {
				l3TotalRtt += probe.RttMs
				l3SuccessCount++
			}
			if !haveL3 {
				summary.L3Healthy, haveL3 = probe.Success, true
			}
		case "l4_port":
			if !haveL4 {
				summary.L4Healthy, haveL4 = probe.Success, true
			}
		case "agent_telemetry":
			if !haveL3 {
				summary.L3Healthy, haveL3 = probe.Success, true
			}
			if !haveL4 {
				summary.L4Healthy, haveL4 = probe.Success, true
			}
		}
	}

	if l3SuccessCount > 0 {
		summary.AvgRttMs = l3TotalRtt / float64(l3SuccessCount)
	}
	if l3TotalProbes > 0 {
		summary.LossRate = float64(l3LossCount) / float64(l3TotalProbes)
	}

	// Detect firewall protection: probe succeeds (agent took over) but detail mentions "firewall"
	// OR: direct probe fails but agent is healthy (legacy path)
	var firewallDetected bool
	for _, probe := range probes {
		if strings.Contains(strings.ToLower(probe.Detail), "firewall") {
			firewallDetected = true
			break
		}
	}
	if firewallDetected || ((!summary.L3Healthy || !summary.L4Healthy) && agentHealthy) {
		summary.FirewallProtected = true
		summary.AgentHealthy = true // Agent was healthy when firewall protection was recorded
		summary.LatencySource = "ws"
		if hasTel && tel.WSRttMs > 0 {
			summary.AvgRttMs = tel.WSRttMs
		} else if node.WSRttMs > 0 {
			summary.AvgRttMs = node.WSRttMs
		}
		summary.LossRate = 0.0
		summary.DataHealthy = true
		summary.OverallState = "healthy"
	} else if summary.L3Healthy && summary.L4Healthy {
		summary.DataHealthy = true
		summary.OverallState = "healthy"
		summary.LatencySource = "icmp"
	} else {
		summary.DataHealthy = false
		if node.Status == "offline" || !agentHealthy {
			summary.OverallState = "critical"
		} else {
			summary.OverallState = "degraded"
		}
	}

	if len(probes) > 0 {
		summary.LastChecked = probes[0].RecordedAt.Format("15:04:05")
	} else {
		summary.LastChecked = time.Now().Format("15:04:05")
	}

	return summary
}
