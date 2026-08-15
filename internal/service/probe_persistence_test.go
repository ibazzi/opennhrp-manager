package service

import (
	"testing"
	"time"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
)

func TestProbePersistenceSamplesStableStateAndKeepsTransitions(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	witness := NewWitnessService(&config.Config{}, database, nil)
	start := time.Now().Add(-time.Minute)
	probe := db.WitnessProbeRecord{
		TargetNodeID: "hub-a",
		ProbeType:    "l3_nbma",
		TargetIP:     "192.0.2.1",
		RttMs:        10,
		Success:      true,
		Detail:       "ICMP ping",
		RecordedAt:   start,
	}

	if err := witness.saveProbe(probe); err != nil {
		t.Fatal(err)
	}
	probe.RttMs = 20
	probe.RecordedAt = start.Add(5 * time.Second)
	if err := witness.saveProbe(probe); err != nil {
		t.Fatal(err)
	}
	assertProbeCount(t, database, 1)

	probe.LossRate = 0.5
	probe.RecordedAt = start.Add(10 * time.Second)
	if err := witness.saveProbe(probe); err != nil {
		t.Fatal(err)
	}
	probe.Success = false
	probe.LossRate = 1
	probe.RecordedAt = start.Add(11 * time.Second)
	if err := witness.saveProbe(probe); err != nil {
		t.Fatal(err)
	}
	probe.Detail = "ICMP blocked by firewall"
	probe.RecordedAt = start.Add(12 * time.Second)
	if err := witness.saveProbe(probe); err != nil {
		t.Fatal(err)
	}
	probe.TargetIP = "192.0.2.2"
	probe.RecordedAt = start.Add(13 * time.Second)
	if err := witness.saveProbe(probe); err != nil {
		t.Fatal(err)
	}
	probe.RttMs = 30
	probe.RecordedAt = start.Add(43 * time.Second)
	if err := witness.saveProbe(probe); err != nil {
		t.Fatal(err)
	}
	assertProbeCount(t, database, 6)
}

func TestProbePersistenceRetriesFailedWrites(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	witness := NewWitnessService(&config.Config{}, database, nil)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	probe := db.WitnessProbeRecord{TargetNodeID: "hub-a", ProbeType: "l4_port", Success: true}
	if err := witness.saveProbe(probe); err == nil {
		t.Fatal("closed database write unexpectedly succeeded")
	}
	if len(witness.persistedProbes) != 0 {
		t.Fatal("failed write advanced persistence state")
	}

	retryDB, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer retryDB.Close()
	witness.database = retryDB
	if err := witness.saveProbe(probe); err != nil {
		t.Fatal(err)
	}
	assertProbeCount(t, retryDB, 1)
}

func assertProbeCount(t *testing.T, database *db.DB, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT COUNT(*) FROM witness_probes`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("probe count = %d, want %d", got, want)
	}
}
