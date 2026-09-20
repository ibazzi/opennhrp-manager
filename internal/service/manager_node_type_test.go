package service

import (
	"path/filepath"
	"testing"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
)

func TestHubExecutorNeverSelectsSpoke(t *testing.T) {
	database, err := db.InitDB(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	manager := NewNodeManager(&config.Config{}, database, NewLogHub())
	hub := executor.NewAgentExecutor("hub-1", "hub", nil)
	spoke := executor.NewAgentExecutor("branch-1", "spoke", nil)
	manager.agents[hub.GetNodeID()] = hub
	manager.agents[spoke.GetNodeID()] = spoke

	got, err := manager.GetHubExecutor("")
	if err != nil || got.GetNodeID() != "hub-1" {
		t.Fatalf("auto Hub routing selected %v, err=%v", got, err)
	}
	if _, err := manager.GetHubExecutor("branch-1"); err == nil {
		t.Fatal("explicit Spoke was accepted by Hub routing")
	}

	newConn := executor.NewAgentExecutor("branch-1", "spoke", nil)
	manager.agents["branch-1"] = newConn
	manager.UnregisterAgent("branch-1", spoke)
	if manager.agents["branch-1"] != newConn {
		t.Fatal("stale disconnect removed replacement Agent")
	}
}

func TestOpenNHRPExecutorAcceptsHubAndSpokeOnly(t *testing.T) {
	database, err := db.InitDB(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	manager := NewNodeManager(&config.Config{}, database, NewLogHub())
	manager.agents["hub-1"] = executor.NewAgentExecutor("hub-1", "hub", nil)
	manager.agents["branch-1"] = executor.NewAgentExecutor("branch-1", "spoke", nil)
	manager.agents["witness-1"] = executor.NewAgentExecutor("witness-1", "witness", nil)

	for _, id := range []string{"hub-1", "branch-1"} {
		got, err := manager.GetOpenNHRPExecutor(id)
		if err != nil || got.GetNodeID() != id {
			t.Fatalf("OpenNHRP node %s rejected: got=%v err=%v", id, got, err)
		}
	}
	for _, id := range []string{"witness-1", "offline"} {
		if _, err := manager.GetOpenNHRPExecutor(id); err == nil {
			t.Fatalf("non-OpenNHRP node %s was accepted", id)
		}
	}
}
