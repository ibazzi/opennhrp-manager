package executor

import (
	"testing"
	"time"
)

func TestCurrentOpenNHRPOutputs(t *testing.T) {
	cluster, err := ParseClusterStatusFromJSON(`Status: ok

{"cluster_id":"0123456789abcdef0123456789abcdef","primary":"tunnel","leader":"tunnel","local_member":"orcl-jp1","term":62,"commit_index":59,"manifest_revision":9,"digest":"abc","service_available":true,"isolated":false,"network_health":"disabled","health_interval_seconds":10,"health_failure_rounds":2,"health_recovery_rounds":3,"health_targets":[],"witness":{"capable":true,"mode":"active","epoch":"00112233445566778899aabbccddeeff","peer_vote":true,"manager_vote":false,"quorum_available":true,"lease_holder":"tunnel","lease_term":62,"lease_sequence":8,"lease_remaining_ms":2500,"fallback_remaining_ms":0},"members":[{"member":"tunnel","addresses":[{"address":"114.28.143.35","origin":"configured"},{"address":"198.51.100.2","origin":"observed"}],"priority":100,"state":"active","connected":true,"authenticated":true,"match_index":5201},{"member":"orcl-jp1","addresses":[{"address":"10.0.0.3","origin":"configured"}],"priority":90,"state":"active","connected":false,"authenticated":false,"match_index":5228}]}`, "agent", "")
	if err != nil {
		t.Fatal(err)
	}
	if cluster.ClusterID == "" || cluster.Primary != "tunnel" || cluster.Member != "orcl-jp1" || cluster.LocalRole != "follower" || cluster.NetworkHealthStatus != "disabled" || cluster.HealthFailureRounds != 2 || cluster.HealthRecoveryRounds != 3 {
		t.Fatalf("unexpected cluster status: %+v", cluster)
	}
	standby, err := ParseClusterStatusFromJSON(`{"leader":"hub-primary","local_member":"hub-backup","service_available":true,"witness":{"mode":"active","quorum_available":false}}`, "agent", "")
	if err != nil {
		t.Fatal(err)
	}
	if standby.LocalRole != "standby" {
		t.Fatalf("unexpected unavailable follower role: %+v", standby)
	}
	if cluster.Members[0].Observed != "198.51.100.2" || cluster.Members[0].Lag != 27 || !cluster.Members[0].Authenticated {
		t.Fatalf("unexpected member conversion: %+v", cluster.Members[0])
	}
	if cluster.Digest != "abc" || !cluster.Witness.Capable || cluster.Witness.Mode != "active" || cluster.Witness.LeaseSequence != 8 {
		t.Fatalf("unexpected witness status: %+v", cluster.Witness)
	}

	replication, err := ParseReplicationStatusFromJSON(`Status: ok

{"local_index":5228,"digest":"abc","snapshots_sent":1,"deltas_sent":2,"snapshots_received":3,"deltas_received":4,"resync_requests":5,"peers":[{"member":"tunnel","match_index":5201,"lag":27,"digest":"def","connected":true}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if replication.LocalIndex != 5228 || replication.DeltasReceived != 4 || replication.Peers[0].MemberID != "tunnel" || replication.Peers[0].Lag != 27 {
		t.Fatalf("unexpected replication status: %+v", replication)
	}

	invites, err := ParseInviteListFromJSON(`{"invites":[{"id_prefix":"3516a5725dc2","member":"orcl-jp1","priority":90,"expires_at":1786629075,"state":"claimed"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if invites[0].MemberID != "orcl-jp1" || !invites[0].ExpiresAt.Equal(time.Unix(1786629075, 0)) {
		t.Fatalf("unexpected invite: %+v", invites[0])
	}

	keys, err := ParseKeyStatusFromJSON(`Status: ok

{"current_key_id":"current","next_key_id":"next"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !keys.HasNextKey || keys.NextKeyID != "next" {
		t.Fatalf("unexpected key status: %+v", keys)
	}

	ha, err := ParseHAStatusFromJSON(`Status: ok

{"event_sequence":7,"interface":"gre-ha","mode":"managed","coordinator_state":"running","coordinator_last_exit":0,"protocol":"10.0.0.3","prefix_length":32,"generation":4,"hub_list_generation":8,"hub_list_source":"hub-primary","switching":false,"auth_mode":"required","auth_cluster_id":"cluster","seen_term":12,"seen_commit_index":34,"seen_leader":"hub-primary","current_key_id":"current","next_key_id":"next","active_member":"hub-primary","candidates":[{"member":"hub-primary","nbma":"198.51.100.1","addresses":["198.51.100.1"],"endpoint_reachable":[true],"selected_address":"198.51.100.1","priority":100,"local_nbma":null,"local_nbma_origin":null,"origin":"static","state":"ready","registered":true,"ready":true,"active":true,"authenticated":true,"auth_key_id":"current","term":12,"commit_index":34,"leader":"hub-primary","srtt_ms":12.5,"rto_ms":50,"loss_pct":1.25,"score":96}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if ha.Interface != "gre-ha" || ha.EventSequence != 7 || ha.ActiveMember == nil || *ha.ActiveMember != "hub-primary" || len(ha.Candidates) != 1 || ha.Candidates[0].Score != 96 || ha.Candidates[0].LossPct != 1.25 {
		t.Fatalf("unexpected HA status: %+v", ha)
	}
	if _, err := ParseHAStatusFromJSON(`Status: ok

{"error":"service-not-found"}`); err == nil {
		t.Fatal("expected HA status error")
	}
}
