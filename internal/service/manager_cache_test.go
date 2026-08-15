package service

import (
	"testing"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/executor"
)

func TestNodeManagerCachesLastSuccessfulHubView(t *testing.T) {
	mgr := NewNodeManager(&config.Config{}, nil, nil)
	status := &executor.ClusterStatusInfo{
		Member:  "hub-primary",
		Members: []executor.MemberInfo{{MemberID: "hub-primary"}},
	}
	spokes := []executor.SpokeInfo{{ProtocolAddress: "10.20.0.2/32"}}

	mgr.CacheClusterStatus("hub-primary", status)
	mgr.CacheSpokes("hub-primary", "", spokes)

	cachedStatus, ok := mgr.GetCachedClusterStatus("hub-primary")
	if !ok || cachedStatus.Member != "hub-primary" || len(cachedStatus.Members) != 1 {
		t.Fatalf("unexpected cached cluster status: %#v", cachedStatus)
	}
	cachedSpokes, ok := mgr.GetCachedSpokes("hub-primary", "")
	if !ok || len(cachedSpokes) != 1 || cachedSpokes[0].ProtocolAddress != "10.20.0.2/32" {
		t.Fatalf("unexpected cached spokes: %#v", cachedSpokes)
	}
}
