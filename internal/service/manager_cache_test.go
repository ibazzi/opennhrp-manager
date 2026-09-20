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
	mgr.CacheSpokes("hub-secondary", "gre-ha", spokes)
	byNode := mgr.GetCachedSpokesByNode()
	if len(byNode["hub-primary"]) != 1 || byNode["hub-secondary"] != nil {
		t.Fatalf("unexpected spokes by node: %#v", byNode)
	}
	replication := &executor.ReplicationStatusInfo{
		LocalIndex: 7,
		Peers:      []executor.ReplicationPeerInfo{{MemberID: "hub-secondary", Lag: 1}},
	}
	invites := []executor.InviteRecord{{IDPrefix: "abc123", MemberID: "hub-new"}}
	keyStatus := &executor.KeyStatusInfo{CurrentKeyID: "key-1"}
	mgr.CacheHAStatus("hub-primary", replication, invites, keyStatus)
	gotReplication, gotInvites, gotKeyStatus := mgr.GetCachedHAStatus("hub-primary")
	if gotReplication == nil || gotReplication.LocalIndex != 7 || len(gotReplication.Peers) != 1 || len(gotInvites) != 1 || gotKeyStatus == nil || gotKeyStatus.CurrentKeyID != "key-1" {
		t.Fatalf("unexpected cached HA status: %v, %#v, %#v", gotReplication, gotInvites, gotKeyStatus)
	}
}
