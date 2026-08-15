package test

import (
	"context"
	"testing"
	"time"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/service"
)

func TestSummarizeNodeSLAUsesLatestProbePerLayer(t *testing.T) {
	now := time.Unix(100, 0)
	probes := []db.WitnessProbeRecord{
		{ProbeType: "l4_port", Success: false, RecordedAt: now},
		{ProbeType: "l3_nbma", Success: true, RttMs: 31.25, RecordedAt: now.Add(-time.Second)},
		{ProbeType: "l4_port", Success: true, RttMs: 30, RecordedAt: now.Add(-2 * time.Second)},
	}
	node := db.NodeRecord{ID: "hub", Status: "online"}
	summary := service.SummarizeNodeSLA("hub", probes, node, service.AgentTelemetry{}, false, false)
	if !summary.L3Healthy || summary.L4Healthy || summary.DataHealthy || summary.OverallState != "critical" || summary.AvgRttMs != 31.25 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if service.IsNodeProbeHealthy(probes) {
		t.Fatal("latest L4 failure must make the direct probe unhealthy")
	}
}

func TestSummarizeNodeSLAFirewallProtected(t *testing.T) {
	now := time.Now()
	// Inbound probes all failed (firewall blocked)
	probes := []db.WitnessProbeRecord{
		{ProbeType: "l4_port", Success: false, LossRate: 1.0, RecordedAt: now},
		{ProbeType: "l3_nbma", Success: false, LossRate: 1.0, RecordedAt: now.Add(-time.Second)},
	}
	node := db.NodeRecord{ID: "primary-hub", Status: "online", WSRttMs: 14.5}
	tel := service.AgentTelemetry{
		LastHeartbeat: now,
		WSRttMs:       14.5,
		NetworkHealth: true,
		ServiceAvail:  true,
		ActiveSpokes:  5,
	}

	// When agent is healthy
	summary := service.SummarizeNodeSLA("primary-hub", probes, node, tel, true, true)
	if !summary.FirewallProtected {
		t.Fatal("expected FirewallProtected to be true")
	}
	if !summary.DataHealthy || summary.OverallState != "healthy" {
		t.Fatalf("expected healthy overall state, got: %+v", summary)
	}
	if summary.LatencySource != "ws" || summary.AvgRttMs != 14.5 {
		t.Fatalf("expected WS latency of 14.5ms, got: %f (%s)", summary.AvgRttMs, summary.LatencySource)
	}
	if summary.LossRate != 0.0 {
		t.Fatalf("expected 0.0 loss rate under agent health, got: %f", summary.LossRate)
	}
	if summary.ActiveSpokes != 5 {
		t.Fatalf("expected 5 active spokes, got: %d", summary.ActiveSpokes)
	}
}

func TestArbitrationFirewallTolerance(t *testing.T) {
	cfg := &config.Config{
		WitnessEnabled:  true,
		WitnessInterval: 5,
	}
	nodeMgr := service.NewNodeManager(cfg, nil, nil)
	now := time.Now()

	nodeMgr.SetTelemetryForTest("hub-primary", service.AgentTelemetry{
		LastHeartbeat: now,
		WSRttMs:       12.0,
		NetworkHealth: true,
		ServiceAvail:  true,
	})
	nodeMgr.SetAgentConnectedForTest("hub-primary", true)

	nodeMgr.SetTelemetryForTest("hub-backup", service.AgentTelemetry{
		LastHeartbeat: now,
		WSRttMs:       15.0,
		NetworkHealth: true,
		ServiceAvail:  true,
	})
	nodeMgr.SetAgentConnectedForTest("hub-backup", true)

	w := service.NewWitnessService(cfg, nil, nodeMgr)

	// 1. Direct probes failed on primary due to firewall
	primaryProbes := []db.WitnessProbeRecord{
		{ProbeType: "l3_nbma", Success: false},
		{ProbeType: "l4_port", Success: false},
	}
	// Direct probes ok on backup
	backupProbes := []db.WitnessProbeRecord{
		{ProbeType: "l3_nbma", Success: true},
		{ProbeType: "l4_port", Success: true},
	}

	if !w.IsNodeHealthy("hub-primary", primaryProbes) {
		t.Fatal("hub-primary should be recognized healthy via inband Agent telemetry")
	}
	if !w.IsNodeHealthy("hub-backup", backupProbes) {
		t.Fatal("hub-backup should be recognized healthy via direct probes")
	}

	// 2. Both nodes behind firewall
	backupProbesFW := []db.WitnessProbeRecord{
		{ProbeType: "l3_nbma", Success: false},
		{ProbeType: "l4_port", Success: false},
	}
	if !w.IsNodeHealthy("hub-backup", backupProbesFW) {
		t.Fatal("hub-backup should also be recognized healthy via inband Agent telemetry when behind firewall")
	}

	// 3. Primary actually dies (agent disconnects / fails heartbeats)
	nodeMgr.SetAgentConnectedForTest("hub-primary", false)
	if w.IsNodeHealthy("hub-primary", primaryProbes) {
		t.Fatal("hub-primary should be unhealthy when agent is disconnected and probes fail")
	}
}

func TestArbitrationWithInMemoryDB(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatalf("failed to init in-memory db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		WitnessEnabled:  true,
		WitnessInterval: 1,
	}
	nodeMgr := service.NewNodeManager(cfg, database, nil)
	now := time.Now()

	nodeMgr.SetTelemetryForTest("primary-1", service.AgentTelemetry{
		LastHeartbeat: now,
		WSRttMs:       11.2,
		NetworkHealth: true,
		ServiceAvail:  true,
	})
	nodeMgr.SetAgentConnectedForTest("primary-1", true)

	w := service.NewWitnessService(cfg, database, nodeMgr)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	w.Start(ctx)
}
