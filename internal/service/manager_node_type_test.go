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
