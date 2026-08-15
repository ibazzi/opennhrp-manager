package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/protocol"
)

func testWitnessHub(nodeID, member, primary, leader, mode, epoch string,
	term, index uint64, digest string, now time.Time) witnessHub {
	return witnessHub{NodeID: nodeID, AgentTelemetry: AgentTelemetry{
		LastHeartbeat: now, AgentConnected: true, ClusterID: "cluster-1",
		MemberID: member, Primary: primary, Leader: leader, ServiceAvail: true,
		LocalRole: "standby", Term: term, CommitIndex: index, Digest: digest,
		Witness: protocol.WitnessPayload{Capable: true, Mode: mode, Epoch: epoch},
	}}
}

func TestWitnessHolderSelection(t *testing.T) {
	now := time.Now()
	primary := testWitnessHub("node-a", "hub-a", "hub-a", "hub-a", "active", "epoch", 5, 9, "same", now)
	backup := testWitnessHub("node-b", "hub-b", "hub-a", "hub-a", "active", "epoch", 5, 9, "same", now)
	holder, term, _, ok := selectWitnessHolder([]witnessHub{backup, primary}, db.WitnessClusterRecord{}, now)
	if !ok || holder.MemberID != "hub-a" || term != 5 {
		t.Fatalf("fixed Primary tie-break failed: holder=%s term=%d ok=%v", holder.MemberID, term, ok)
	}
	primary.Leader = "hub-b"
	backup.Leader = "hub-b"
	holder, term, reason, ok := selectWitnessHolder([]witnessHub{backup, primary}, db.WitnessClusterRecord{}, now)
	if !ok || holder.MemberID != "hub-b" || term != 5 || !strings.Contains(reason, "agree") {
		t.Fatalf("agreed current Leader was not retained: holder=%s term=%d ok=%v reason=%q", holder.MemberID, term, ok, reason)
	}
	primary.Leader = "hub-a"
	backup.Leader = "hub-a"

	record := db.WitnessClusterRecord{Holder: "hub-a", Term: 5, SafeUntil: now.Add(time.Second)}
	backup.Term = 7
	holder, term, _, ok = selectWitnessHolder([]witnessHub{primary, backup}, record, now)
	if !ok || holder.MemberID != "hub-a" || term != 5 {
		t.Fatalf("healthy holder was not retained: holder=%s term=%d ok=%v", holder.MemberID, term, ok)
	}

	record = db.WitnessClusterRecord{Holder: "hub-b", Term: 5, SafeUntil: now.Add(time.Second)}
	backup.Leader = "hub-a"
	backup.Term = 5
	if _, _, reason, ok := selectWitnessHolder([]witnessHub{primary, backup}, record, now); ok || !strings.Contains(reason, "safety") {
		t.Fatalf("coordinated failback changed holder before safety deadline: ok=%v reason=%q", ok, reason)
	}
	holder, term, _, ok = selectWitnessHolder([]witnessHub{primary, backup}, record, now.Add(2*time.Second))
	if !ok || holder.MemberID != "hub-a" || term != 5 {
		t.Fatalf("coordinated failback did not move holder: holder=%s term=%d ok=%v", holder.MemberID, term, ok)
	}

	primary.ServiceAvail = false
	if _, _, reason, ok := selectWitnessHolder([]witnessHub{primary, backup}, record, now); ok || !strings.Contains(reason, "safety") {
		t.Fatalf("old holder safety deadline was not enforced: ok=%v reason=%q", ok, reason)
	}

	record = db.WitnessClusterRecord{}
	primary.ServiceAvail = true
	backup.Term = primary.Term
	primary.Digest = "left"
	backup.Digest = "right"
	if _, _, reason, ok := selectWitnessHolder([]witnessHub{primary, backup}, record, now); ok || !strings.Contains(reason, "digest") {
		t.Fatalf("digest conflict was not rejected: ok=%v reason=%q", ok, reason)
	}
}

func TestWitnessCommandTimeoutAndTermConflictGate(t *testing.T) {
	now := time.Now()
	primary := testWitnessHub("node-a", "hub-a", "hub-a", "hub-a", "active", "epoch", 34, 9, "same", now)
	backup := testWitnessHub("node-b", "hub-b", "hub-a", "hub-a", "active", "epoch", 34, 9, "same", now)
	if got := witnessCommandTimeout([]witnessHub{primary, backup}); got != witnessCommandMin {
		t.Fatalf("normal RTT timeout=%s, want %s", got, witnessCommandMin)
	}
	primary.WSRttMs = 487
	if got := witnessCommandTimeout([]witnessHub{primary, backup}); got != 1224*time.Millisecond {
		t.Fatalf("adaptive timeout=%s, want 1.224s", got)
	}
	primary.WSRttMs = 881
	if got := witnessCommandTimeout([]witnessHub{primary, backup}); got != witnessCommandMax {
		t.Fatalf("capped timeout=%s, want %s", got, witnessCommandMax)
	}

	decision, term := witnessLeaseFailure(&executor.AgentCommandError{Detail: "Status: error\nReason: witness-lease-rejected\nCurrent-Term: 34\nCurrent-Leader: hub-a"})
	if decision != "witness_lease_term_conflict" || term != 34 {
		t.Fatalf("term conflict classification=%s term=%d", decision, term)
	}
	if decision, _ := witnessLeaseFailure(context.DeadlineExceeded); decision != "witness_lease_command_timeout" {
		t.Fatalf("deadline classification=%s", decision)
	}

	witness := NewWitnessService(&config.Config{}, nil, nil)
	witness.termFloor["cluster-1"] = 34
	backup.Term = 33
	if witness.termViewsConverged("cluster-1", []witnessHub{primary, backup}) {
		t.Fatal("stale Hub term passed convergence gate")
	}
	backup.Term = 34
	if !witness.termViewsConverged("cluster-1", []witnessHub{primary, backup}) {
		t.Fatal("matching fresh Hub terms did not clear convergence gate")
	}
}

func TestMemberDisableRequiresConvergedRemainingPair(t *testing.T) {
	now := time.Now()
	mgr := NewNodeManager(&config.Config{WitnessEnabled: true}, nil, nil)
	status := &executor.ClusterStatusInfo{
		ClusterID: "cluster-1", Term: 7, CommitIndex: 12, Leader: "hub-b", Digest: "same",
		Members: []executor.MemberInfo{
			{MemberID: "hub-a", State: "active"},
			{MemberID: "hub-b", State: "active"},
			{MemberID: "hub-c", State: "active"},
		},
	}
	backup := testWitnessHub("node-b", "hub-b", "hub-a", "hub-b", "legacy", "", 7, 12, "same", now)
	backup.MemberState = "active"
	mgr.SetTelemetryForTest(backup.NodeID, backup.AgentTelemetry)
	mgr.SetAgentConnectedForTest(backup.NodeID, true)
	if err := mgr.ValidateMemberDisable(status, "hub-c"); err == nil {
		t.Fatal("disable was allowed while one remaining Hub was offline")
	}
	primary := testWitnessHub("node-a", "hub-a", "hub-a", "hub-b", "legacy", "", 7, 12, "same", now)
	primary.MemberState = "active"
	mgr.SetTelemetryForTest(primary.NodeID, primary.AgentTelemetry)
	mgr.SetAgentConnectedForTest(primary.NodeID, true)
	if err := mgr.ValidateMemberDisable(status, "hub-c"); err != nil {
		t.Fatalf("converged remaining pair was rejected: %v", err)
	}
	primary.Digest = "different"
	mgr.SetTelemetryForTest(primary.NodeID, primary.AgentTelemetry)
	if err := mgr.ValidateMemberDisable(status, "hub-c"); err == nil {
		t.Fatal("disable was allowed with divergent remaining Hubs")
	}
	if err := NewNodeManager(&config.Config{}, nil, nil).ValidateMemberDisable(status, "hub-c"); err != nil {
		t.Fatalf("legacy mode member change was blocked: %v", err)
	}
}

func TestWitnessAutomaticActivationAndSingleAgentGuard(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now()
	mgr := NewNodeManager(&config.Config{}, database, nil)
	for _, hub := range []witnessHub{
		testWitnessHub("node-a", "hub-a", "hub-a", "hub-a", "legacy", "", 1, 1, "same", now),
		testWitnessHub("node-b", "hub-b", "hub-a", "hub-a", "legacy", "", 1, 1, "same", now),
	} {
		mgr.SetTelemetryForTest(hub.NodeID, hub.AgentTelemetry)
		mgr.SetAgentConnectedForTest(hub.NodeID, true)
	}
	witness := NewWitnessService(&config.Config{}, database, mgr)
	var mu sync.Mutex
	var commands []string
	witness.command = func(_ context.Context, nodeID, command string) error {
		mu.Lock()
		commands = append(commands, nodeID+":"+command)
		mu.Unlock()
		return nil
	}
	witness.runQuorumCycle(context.Background(), now)
	if len(commands) != 4 {
		t.Fatalf("expected prepare+activate on both Hubs, got %v", commands)
	}
	records, err := database.GetWitnessClusters()
	if err != nil || len(records) != 1 || records[0].Mode != "active" || len(records[0].Epoch) != 32 {
		t.Fatalf("unexpected persisted activation: records=%+v err=%v", records, err)
	}
	witness.runQuorumCycle(context.Background(), now.Add(250*time.Millisecond))
	records, err = database.GetWitnessClusters()
	if err != nil || len(records) != 1 || records[0].Mode != "active" {
		t.Fatalf("stale pre-activation telemetry disabled Witness: records=%+v err=%v", records, err)
	}

	database2, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database2.Close()
	mgr2 := NewNodeManager(&config.Config{}, database2, nil)
	one := testWitnessHub("node-a", "hub-a", "hub-a", "hub-a", "legacy", "", 1, 1, "same", now)
	mgr2.SetTelemetryForTest(one.NodeID, one.AgentTelemetry)
	mgr2.SetAgentConnectedForTest(one.NodeID, true)
	witness2 := NewWitnessService(&config.Config{}, database2, mgr2)
	witness2.command = func(_ context.Context, _, _ string) error {
		t.Fatal("single Agent must not activate Witness")
		return nil
	}
	witness2.runQuorumCycle(context.Background(), now)

	one.Witness.Capable = false
	one.Witness.Policy = "legacy"
	mgr2.SetTelemetryForTest(one.NodeID, one.AgentTelemetry)
	record := db.WitnessClusterRecord{
		ClusterID: "cluster-1", Epoch: "old-epoch", Mode: "active",
		Holder: "hub-a", Term: 1,
	}
	if err := database2.SaveWitnessCluster(record); err != nil {
		t.Fatal(err)
	}
	witness2.clusters[record.ClusterID] = record
	witness2.runQuorumCycle(context.Background(), now.Add(time.Second))
	records, err = database2.GetWitnessClusters()
	if err != nil || len(records) != 1 || records[0].Mode != "legacy" ||
		records[0].Transition != "single_hub_legacy" {
		t.Fatalf("single-Hub legacy state did not clear Witness: records=%+v err=%v", records, err)
	}
}

func TestWitnessThreeHubUsesHubMajority(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now()
	mgr := NewNodeManager(&config.Config{}, database, nil)
	for i, member := range []string{"hub-a", "hub-b", "hub-c"} {
		hub := testWitnessHub("node-"+member, member, "hub-a", "hub-a",
			"legacy", "", 1, 1, "same", now)
		hub.Witness.Capable = false
		hub.Witness.Policy = "hub-majority"
		hub.Witness.Voters = 3
		hub.Witness.Required = 2
		hub.Witness.Votes = 3 - i
		mgr.SetTelemetryForTest(hub.NodeID, hub.AgentTelemetry)
		mgr.SetAgentConnectedForTest(hub.NodeID, true)
	}
	witness := NewWitnessService(&config.Config{}, database, mgr)
	witness.command = func(_ context.Context, _, _ string) error {
		t.Fatal("three-Hub majority must not request a Manager lease")
		return nil
	}
	witness.runQuorumCycle(context.Background(), now)
	status := witness.GetQuorumStatus()
	if status.Policy != "hub-majority" || status.Voters != 3 ||
		status.Required != 2 {
		t.Fatalf("unexpected Hub majority status: %+v", status)
	}
}

func TestWitnessLeaseSequenceAndRestartSilence(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now()
	epoch := "00112233445566778899aabbccddeeff"
	mgr := NewNodeManager(&config.Config{}, database, nil)
	for _, hub := range []witnessHub{
		testWitnessHub("node-a", "hub-a", "hub-a", "hub-a", "active", epoch, 3, 7, "same", now),
		testWitnessHub("node-b", "hub-b", "hub-a", "hub-a", "active", epoch, 3, 7, "same", now),
	} {
		hub.Witness.Policy = "manager-witness"
		hub.Witness.Voters = 3
		mgr.SetTelemetryForTest(hub.NodeID, hub.AgentTelemetry)
		mgr.SetAgentConnectedForTest(hub.NodeID, true)
	}
	record := db.WitnessClusterRecord{ClusterID: "cluster-1", Epoch: epoch, Mode: "active", Sequence: 7}
	if err := database.SaveWitnessCluster(record); err != nil {
		t.Fatal(err)
	}
	witness := NewWitnessService(&config.Config{}, database, mgr)
	witness.clusters[record.ClusterID] = record
	var mu sync.Mutex
	var commands []string
	witness.command = func(_ context.Context, _, command string) error {
		mu.Lock()
		commands = append(commands, command)
		mu.Unlock()
		return nil
	}
	witness.runQuorumCycle(context.Background(), now)
	if len(commands) != 2 || !strings.Contains(commands[0], "sequence 8") || !strings.Contains(commands[0], "holder hub-a") {
		t.Fatalf("unexpected first lease commands: %v", commands)
	}
	witness.runQuorumCycle(context.Background(), now.Add(500*time.Millisecond))
	if len(commands) != 2 {
		t.Fatalf("lease renewed before one second: %v", commands)
	}

	witness.startedAt = now
	witness.lastAction[record.ClusterID] = time.Time{}
	witness.runQuorumCycle(context.Background(), now.Add(time.Second))
	if len(commands) != 2 {
		t.Fatalf("restart silence did not suppress lease: %v", commands)
	}

	witness.startedAt = time.Time{}
	witness.lastAction[record.ClusterID] = time.Time{}
	leaseSent := make(chan struct{}, 2)
	witness.command = func(_ context.Context, _, _ string) error {
		leaseSent <- struct{}{}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time, 1)
	done := make(chan struct{})
	go func() {
		witness.runQuorumLoop(ctx, ticks)
		close(done)
	}()
	ticks <- time.Now()
	select {
	case <-leaseSent:
	case <-time.After(time.Second):
		t.Fatal("dedicated quorum loop did not renew the lease")
	}
	cancel()
	<-done
}

func TestWitnessRestartDoesNotPromoteFirstReconnectingBackup(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now()
	epoch := "00112233445566778899aabbccddeeff"
	record := db.WitnessClusterRecord{
		ClusterID: "cluster-1", Epoch: epoch, Mode: "active",
		Holder: "hub-a", Term: 3, Sequence: 7,
	}
	if err := database.SaveWitnessCluster(record); err != nil {
		t.Fatal(err)
	}
	mgr := NewNodeManager(&config.Config{}, database, nil)
	backup := testWitnessHub("node-b", "hub-b", "hub-a", "hub-a", "active", epoch, 3, 7, "same", now)
	backup.Witness.Policy = "manager-witness"
	mgr.SetTelemetryForTest(backup.NodeID, backup.AgentTelemetry)
	mgr.SetAgentConnectedForTest(backup.NodeID, true)

	witness := NewWitnessService(&config.Config{}, database, mgr)
	witness.startedAt = now
	witness.clusters[record.ClusterID] = record
	var commands []string
	witness.command = func(_ context.Context, _, command string) error {
		commands = append(commands, command)
		return nil
	}

	backup.LastHeartbeat = now.Add(4 * time.Second)
	mgr.SetTelemetryForTest(backup.NodeID, backup.AgentTelemetry)
	witness.runQuorumCycle(context.Background(), backup.LastHeartbeat)
	if len(commands) != 0 {
		t.Fatalf("Backup was promoted during Manager restart recovery: %v", commands)
	}

	backup.LastHeartbeat = now.Add(witnessRestartTakeover)
	mgr.SetTelemetryForTest(backup.NodeID, backup.AgentTelemetry)
	witness.runQuorumCycle(context.Background(), backup.LastHeartbeat)
	if len(commands) != 1 || !strings.Contains(commands[0], "term 4 holder hub-b sequence 8") {
		t.Fatalf("Backup was not promoted after restart takeover delay: %v", commands)
	}
}

func TestWitnessEpochReprepare(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now()
	mgr := NewNodeManager(&config.Config{}, database, nil)
	for _, hub := range []witnessHub{
		testWitnessHub("node-a", "hub-a", "hub-a", "hub-a", "active", "old-epoch", 3, 7, "same", now),
		testWitnessHub("node-b", "hub-b", "hub-a", "hub-a", "active", "old-epoch", 3, 7, "same", now),
	} {
		mgr.SetTelemetryForTest(hub.NodeID, hub.AgentTelemetry)
		mgr.SetAgentConnectedForTest(hub.NodeID, true)
	}
	record := db.WitnessClusterRecord{ClusterID: "cluster-1", Epoch: "new-epoch", Mode: "active"}
	witness := NewWitnessService(&config.Config{}, database, mgr)
	witness.clusters[record.ClusterID] = record
	var mu sync.Mutex
	var commands []string
	witness.command = func(_ context.Context, _, command string) error {
		mu.Lock()
		defer mu.Unlock()
		commands = append(commands, command)
		return nil
	}
	witness.runQuorumCycle(context.Background(), now)
	if len(commands) != 4 || !strings.Contains(commands[0], "prepare epoch new-epoch") ||
		!strings.Contains(commands[2], "activate epoch new-epoch") {
		t.Fatalf("epoch conflict was not re-prepared: %v", commands)
	}
}

func TestWitnessFreshnessAndAuditDedup(t *testing.T) {
	now := time.Now()
	fresh := testWitnessHub("node-a", "hub-a", "hub-a", "hub-a", "active", "epoch", 1, 1, "same", now)
	stale := testWitnessHub("node-b", "hub-b", "hub-a", "hub-a", "active", "epoch", 1, 1, "same", now.Add(-2*time.Second))
	if hubs := freshWitnessHubs([]witnessHub{fresh, stale}, now); len(hubs) != 1 || hubs[0].MemberID != "hub-a" {
		t.Fatalf("stale Agent was not excluded: %+v", hubs)
	}
	learner := testWitnessHub("node-c", "hub-c", "hub-a", "hub-a", "active", "epoch", 1, 1, "same", now)
	learner.MemberState = "learner"
	if hubs := freshClusterHubs([]witnessHub{fresh, learner}, now); len(hubs) != 1 || hubs[0].MemberID != "hub-a" {
		t.Fatalf("Learner Agent was counted as an online Hub: %+v", hubs)
	}
	duplicate := fresh
	duplicate.NodeID = "node-duplicate"
	if hubs := freshWitnessHubs([]witnessHub{fresh, duplicate}, now); hubs != nil {
		t.Fatalf("duplicate member telemetry must be rejected: %+v", hubs)
	}

	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	witness := NewWitnessService(&config.Config{}, database, nil)
	record := db.WitnessClusterRecord{ClusterID: "cluster-1", Mode: "active", Holder: "hub-a", Term: 1}
	witness.auditDecision(record, []witnessHub{fresh}, "witness_lease_granted", "selected")
	witness.auditDecision(record, []witnessHub{fresh}, "witness_lease_granted", "selected")
	audits, err := database.GetArbitrations(10)
	if err != nil || len(audits) != 1 || len(audits[0].InvolvedNodeIDs) != 1 ||
		audits[0].InvolvedNodeIDs[0] != "node-a" {
		t.Fatalf("duplicate audit was persisted: audits=%+v err=%v", audits, err)
	}
}
