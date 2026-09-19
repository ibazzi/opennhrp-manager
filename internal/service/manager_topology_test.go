package service

import (
	"encoding/json"
	"testing"
	"time"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/protocol"
)

func TestHeartbeatPublishesTopologySnapshot(t *testing.T) {
	mgr := NewNodeManager(&config.Config{}, nil, nil)
	mgr.lastPersist["hub-primary"] = time.Now()
	mgr.CacheSpokes("hub-primary", "", []executor.SpokeInfo{{
		ProtocolAddress: "10.20.0.2/32", Alias: "branch-a",
	}})
	updates, unsubscribe := mgr.SubscribeTopology()
	defer unsubscribe()

	cluster, _ := json.Marshal(executor.ClusterStatusInfo{Member: "hub-primary", Term: 7})
	spokes, _ := json.Marshal([]executor.SpokeInfo{{ProtocolAddress: "10.20.0.2/32"}})
	mgr.UpdateHeartbeat("hub-primary", protocol.HeartbeatPayload{
		ClusterStatus: cluster,
		Spokes:        spokes,
		Timestamp:     time.Now().Add(-time.Hour),
	}, 56, 0, 1)
	tel, _ := mgr.GetNodeTelemetry("hub-primary")
	if tel.WSRttMs != 56 {
		t.Fatalf("RTT must ignore Agent clock: %v", tel.WSRttMs)
	}

	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Fatal("topology update was not published")
	}
	gotCluster, ok := mgr.GetCachedClusterStatus("hub-primary")
	if !ok || gotCluster.Term != 7 {
		t.Fatalf("unexpected cluster snapshot: %#v", gotCluster)
	}
	gotSpokes, ok := mgr.GetCachedSpokes("hub-primary", "")
	if !ok || len(gotSpokes) != 1 || gotSpokes[0].ProtocolAddress != "10.20.0.2/32" || gotSpokes[0].Alias != "branch-a" {
		t.Fatalf("unexpected spoke snapshot: %#v", gotSpokes)
	}
}

func TestUnavailableWSLatencyDoesNotReuseStoredRTT(t *testing.T) {
	summary := SummarizeNodeSLA("hub", nil, db.NodeRecord{WSRttMs: 80.57}, AgentTelemetry{}, true, true)
	if summary.LatencySource != "ws" || summary.AvgRttMs != 0 {
		t.Fatalf("unavailable measurement reused stored RTT: %+v", summary)
	}
}
