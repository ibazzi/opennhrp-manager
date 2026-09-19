package executor

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSpokeRegistrationMode(t *testing.T) {
	for _, tc := range []struct{ reported, want string }{
		{"ha", "ha"}, {"legacy", "legacy"}, {"", ""}, {"invalid", ""},
	} {
		t.Run(tc.reported, func(t *testing.T) {
			raw := "Type: dynamic\nProtocol-Address: 10.164.0.252/32\n"
			if tc.reported != "" {
				raw += "Registration-Mode: " + tc.reported + "\n"
			}
			raw += "Flags: used up\n\nType: static\nProtocol-Address: 10.164.0.1/32\n"
			spokes := ParseSpokeOutput(raw)
			if len(spokes) != 2 || spokes[0].RegistrationMode != tc.want || spokes[1].RegistrationMode != "" {
				t.Fatalf("unexpected modes: %+v", spokes)
			}
			data, err := json.Marshal(spokes[0])
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatal(err)
			}
			mode, present := payload["registration_mode"]
			if present != (tc.want != "") || (present && mode != tc.want) {
				t.Fatalf("unexpected JSON: %s", data)
			}
		})
	}
}

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

func TestHAQualityRoundTrip(t *testing.T) {
	for _, raw := range []string{
		`{"quality_rtt_ms":20.125,"quality_samples":60,"quality_failures":1,"last_quality_reply_age_ms":150.5,"quality_valid":true,"loss_score":56.666667,"latency_score":27.238636,"priority_score":10,"score":94}`,
		`{"quality_rtt_ms":null,"quality_samples":0,"quality_failures":0,"last_quality_reply_age_ms":null,"quality_valid":false,"loss_score":0,"latency_score":0,"priority_score":10,"score":0}`,
		`{"quality_rtt_ms":20.125,"quality_samples":60,"quality_failures":1,"last_quality_reply_age_ms":150.5,"quality_valid":true,"loss_score":56.666667,"latency_score":27.238636,"priority_score":10,"score":0}`,
	} {
		status, err := ParseHAStatusFromJSON("Status: ok\n\n" + `{"interface":"gre-ha","candidates":[` + raw + `]}`)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(status.Candidates[0])
		if err != nil {
			t.Fatal(err)
		}
		var want, got map[string]any
		if err := json.Unmarshal([]byte(raw), &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &got); err != nil {
			t.Fatal(err)
		}
		for key, value := range want {
			actual, exists := got[key]
			if !exists || actual != value {
				t.Errorf("%s: got %v (present %v), want %v", key, actual, exists, value)
			}
		}
	}
}
