package db

import (
	"math"
	"testing"
	"time"
)

func TestGetProbesAggregatesAndFilters(t *testing.T) {
	database, err := InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().Truncate(time.Second)
	recordedAt := now.Add(-5 * time.Minute)
	for _, probe := range []WitnessProbeRecord{
		{TargetNodeID: "hub-a", ProbeType: "l3_nbma", TargetIP: "192.0.2.1", RttMs: 10, Success: true, RecordedAt: recordedAt},
		{TargetNodeID: "hub-a", ProbeType: "l3_nbma", TargetIP: "192.0.2.1", RttMs: 30, LossRate: 0.25, Success: true, RecordedAt: recordedAt},
		{TargetNodeID: "hub-a", ProbeType: "l3_nbma", TargetIP: "192.0.2.1", LossRate: 1, Success: false, RecordedAt: recordedAt},
		{TargetNodeID: "hub-a", ProbeType: "l4_port", TargetIP: "192.0.2.1:49002", RttMs: 5, Success: true, RecordedAt: recordedAt},
		{TargetNodeID: "hub-b", ProbeType: "l3_nbma", TargetIP: "192.0.2.2", RttMs: 40, Success: true, RecordedAt: recordedAt},
	} {
		if err := database.SaveWitnessProbe(probe); err != nil {
			t.Fatal(err)
		}
	}

	probes, err := database.GetProbes("hub-a", "l3_nbma", 1, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != 1 {
		t.Fatalf("got %d aggregated probes, want 1", len(probes))
	}
	if math.Abs(probes[0].RttMs-20) > 0.001 || probes[0].LossRate != 1 || probes[0].Success {
		t.Fatalf("unexpected aggregate: %+v", probes[0])
	}
}

func TestGetProbesCapsEachSeriesAndKeepsLatestBucket(t *testing.T) {
	database, err := InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().Truncate(time.Second)
	for i := 0; i < 720; i++ {
		if err := database.SaveWitnessProbe(WitnessProbeRecord{
			TargetNodeID: "hub-a",
			ProbeType:    "l3_nbma",
			TargetIP:     "192.0.2.1",
			RttMs:        float64(i + 1),
			Success:      true,
			RecordedAt:   now.Add(-time.Duration(719-i) * 5 * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}

	probes, err := database.GetProbes("hub-a", "l3_nbma", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) > 10 {
		t.Fatalf("got %d points, want at most 10", len(probes))
	}
	latest, err := database.GetProbes("hub-a", "l3_nbma", 1, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(latest) == 0 || now.Sub(latest[len(latest)-1].RecordedAt) > 30*time.Second {
		t.Fatalf("latest bucket is stale: %+v", latest)
	}
}

func TestRecentProbesLimitAppliesPerProbeType(t *testing.T) {
	database, err := InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now()
	if err := database.SaveWitnessProbe(WitnessProbeRecord{TargetNodeID: "hub-a", ProbeType: "l3_nbma", Success: true, RecordedAt: now}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := database.SaveWitnessProbe(WitnessProbeRecord{TargetNodeID: "hub-a", ProbeType: "l4_port", Success: true, RecordedAt: now.Add(time.Duration(i+1) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}

	probes, err := database.GetRecentProbes("hub-a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(probes) != 2 || probes[0].ProbeType != "l4_port" || probes[1].ProbeType != "l3_nbma" {
		t.Fatalf("unexpected recent probes: %+v", probes)
	}
}

func TestCleanupOldProbesUsesLocalCutoff(t *testing.T) {
	database, err := InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now()
	if err := database.SaveWitnessProbe(WitnessProbeRecord{TargetNodeID: "old", ProbeType: "l3_nbma", RecordedAt: now.Add(-25 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := database.SaveWitnessProbe(WitnessProbeRecord{TargetNodeID: "new", ProbeType: "l3_nbma", RecordedAt: now.Add(-23 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := database.CleanupOldProbes(24); err != nil {
		t.Fatal(err)
	}

	var oldCount, newCount int
	if err := database.QueryRow(`SELECT COUNT(*) FROM witness_probes WHERE target_node_id = 'old'`).Scan(&oldCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRow(`SELECT COUNT(*) FROM witness_probes WHERE target_node_id = 'new'`).Scan(&newCount); err != nil {
		t.Fatal(err)
	}
	if oldCount != 0 || newCount != 1 {
		t.Fatalf("cleanup kept old=%d new=%d", oldCount, newCount)
	}
}
