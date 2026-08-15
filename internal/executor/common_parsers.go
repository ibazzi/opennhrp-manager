package executor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type rawOpenNHRPHAStatus struct {
	ClusterID             string `json:"cluster_id"`
	Primary               string `json:"primary"`
	Leader                string `json:"leader"`
	LocalMember           string `json:"local_member"`
	Term                  uint64 `json:"term"`
	CommitIndex           uint64 `json:"commit_index"`
	ManifestRevision      uint64 `json:"manifest_revision"`
	Digest                string `json:"digest"`
	ServiceAvailable      bool   `json:"service_available"`
	Isolated              bool   `json:"isolated"`
	NetworkHealth         any    `json:"network_health"` // string or bool
	HealthIntervalSeconds int    `json:"health_interval_seconds"`
	HealthFailureRounds   int    `json:"health_failure_rounds"`
	HealthRecoveryRounds  int    `json:"health_recovery_rounds"`
	HealthTargets         []struct {
		Address     string `json:"address"`
		TargetIP    string `json:"target_ip"`
		LastSuccess bool   `json:"last_success"`
	} `json:"health_targets"`
	Witness WitnessStatusInfo `json:"witness"`
	Members []struct {
		Member        string `json:"member"`
		MemberID      string `json:"member_id"`
		Priority      int    `json:"priority"`
		State         string `json:"state"`
		Connected     bool   `json:"connected"`
		Authenticated bool   `json:"authenticated"`
		MatchIndex    uint64 `json:"match_index"`
		Addresses     []struct {
			Address string `json:"address"`
			Origin  string `json:"origin"`
		} `json:"addresses"`
	} `json:"members"`
}

func ParseClusterStatusFromJSON(rawText, defaultNodeID, stateDir string) (*ClusterStatusInfo, error) {
	raw, err := ParseJSON[rawOpenNHRPHAStatus](rawText)
	if err != nil {
		return nil, fmt.Errorf("parse cluster json failed: %w", err)
	}

	member := raw.LocalMember
	if member == "" {
		member = defaultNodeID
	}

	info := &ClusterStatusInfo{
		ClusterID:            raw.ClusterID,
		Primary:              raw.Primary,
		Member:               member,
		Leader:               raw.Leader,
		Term:                 raw.Term,
		CommitIndex:          raw.CommitIndex,
		ManifestRevision:     raw.ManifestRevision,
		Digest:               raw.Digest,
		ServiceAvail:         raw.ServiceAvailable,
		HealthIntervalSec:    raw.HealthIntervalSeconds,
		HealthFailureRounds:  raw.HealthFailureRounds,
		HealthRecoveryRounds: raw.HealthRecoveryRounds,
		Isolated:             raw.Isolated,
		StateDir:             stateDir,
		Witness:              raw.Witness,
	}

	// Parse NetworkHealth (string or bool)
	switch v := raw.NetworkHealth.(type) {
	case string:
		info.NetworkHealthStatus = v
		info.NetworkHealth = (v == "healthy")
	case bool:
		info.NetworkHealth = v
		if v {
			info.NetworkHealthStatus = "healthy"
		} else {
			info.NetworkHealthStatus = "unhealthy"
		}
	default:
		info.NetworkHealth = false
		info.NetworkHealthStatus = "unknown"
	}

	// Determine LocalRole
	if raw.Isolated {
		info.LocalRole = "isolated"
	} else if raw.Leader == member && raw.Leader != "" {
		info.LocalRole = "leader"
	} else if raw.Leader != "" {
		info.LocalRole = "standby"
	} else {
		info.LocalRole = "standalone"
	}

	// Convert Members
	for _, m := range raw.Members {
		mID := m.Member
		if mID == "" {
			mID = m.MemberID
		}
		var addrs []string
		var observed string
		for _, a := range m.Addresses {
			if a.Origin == "observed" {
				if observed == "" {
					observed = a.Address
				}
			} else if a.Address != "" {
				addrs = append(addrs, a.Address)
			}
		}

		info.Members = append(info.Members, MemberInfo{
			MemberID:      mID,
			Priority:      m.Priority,
			State:         m.State,
			IsLeader:      (mID == raw.Leader && raw.Leader != ""),
			Advertised:    addrs,
			Observed:      observed,
			MatchIndex:    m.MatchIndex,
			Connected:     m.Connected,
			Authenticated: m.Authenticated,
		})
	}

	// Sort members by Priority descending
	sort.SliceStable(info.Members, func(i, j int) bool {
		if info.Members[i].Priority != info.Members[j].Priority {
			return info.Members[i].Priority > info.Members[j].Priority
		}
		return info.Members[i].MemberID < info.Members[j].MemberID
	})

	var maximumIndex uint64
	for _, member := range info.Members {
		if member.MatchIndex > maximumIndex {
			maximumIndex = member.MatchIndex
		}
	}
	for i := range info.Members {
		info.Members[i].Lag = int64(maximumIndex - info.Members[i].MatchIndex)
	}

	// Convert Health Targets
	for _, t := range raw.HealthTargets {
		addr := t.Address
		if addr == "" {
			addr = t.TargetIP
		}
		info.HealthTargets = append(info.HealthTargets, HealthTargetInfo{
			TargetIP:    addr,
			LastSuccess: t.LastSuccess,
			IntervalSec: raw.HealthIntervalSeconds,
		})
	}

	return info, nil
}

type rawReplicationStatus struct {
	LocalIndex        uint64 `json:"local_index"`
	Digest            string `json:"digest"`
	SnapshotsSent     uint64 `json:"snapshots_sent"`
	DeltasSent        uint64 `json:"deltas_sent"`
	SnapshotsReceived uint64 `json:"snapshots_received"`
	DeltasReceived    uint64 `json:"deltas_received"`
	ResyncRequests    uint64 `json:"resync_requests"`
	Peers             []struct {
		Member     string `json:"member"`
		MatchIndex uint64 `json:"match_index"`
		Lag        int64  `json:"lag"`
		Digest     string `json:"digest"`
		Connected  bool   `json:"connected"`
	} `json:"peers"`
}

func ParseReplicationStatusFromJSON(rawText string) (*ReplicationStatusInfo, error) {
	raw, err := ParseJSON[rawReplicationStatus](rawText)
	if err != nil {
		return nil, fmt.Errorf("parse replication json failed: %w", err)
	}
	info := &ReplicationStatusInfo{
		LocalIndex:        raw.LocalIndex,
		Digest:            raw.Digest,
		SnapshotsSent:     raw.SnapshotsSent,
		DeltasSent:        raw.DeltasSent,
		SnapshotsReceived: raw.SnapshotsReceived,
		DeltasReceived:    raw.DeltasReceived,
		ResyncRequests:    raw.ResyncRequests,
		Peers:             make([]ReplicationPeerInfo, 0, len(raw.Peers)),
	}
	for _, peer := range raw.Peers {
		info.Peers = append(info.Peers, ReplicationPeerInfo{
			MemberID:   peer.Member,
			MatchIndex: peer.MatchIndex,
			Lag:        peer.Lag,
			Digest:     peer.Digest,
			Connected:  peer.Connected,
		})
	}
	return info, nil
}

func ParseInviteListFromJSON(rawText string) ([]InviteRecord, error) {
	type rawInvite struct {
		IDPrefix  string `json:"id_prefix"`
		Member    string `json:"member"`
		Priority  int    `json:"priority"`
		State     string `json:"state"`
		ExpiresAt int64  `json:"expires_at"`
	}
	type rawInviteList struct {
		Invites []rawInvite `json:"invites"`
	}
	raw, err := ParseJSON[rawInviteList](rawText)
	if err != nil {
		return nil, err
	}
	invites := make([]InviteRecord, 0, len(raw.Invites))
	for _, invite := range raw.Invites {
		invites = append(invites, InviteRecord{
			IDPrefix:  invite.IDPrefix,
			MemberID:  invite.Member,
			Priority:  invite.Priority,
			State:     invite.State,
			ExpiresAt: time.Unix(invite.ExpiresAt, 0),
		})
	}
	return invites, nil
}

func ParseKeyStatusFromJSON(rawText string) (*KeyStatusInfo, error) {
	type rawKeyStatus struct {
		CurrentKeyID string  `json:"current_key_id"`
		NextKeyID    *string `json:"next_key_id"`
	}
	raw, err := ParseJSON[rawKeyStatus](rawText)
	if err != nil {
		return nil, err
	}
	info := &KeyStatusInfo{CurrentKeyID: raw.CurrentKeyID}
	if raw.NextKeyID != nil {
		info.NextKeyID = *raw.NextKeyID
		info.HasNextKey = info.NextKeyID != ""
	}
	return info, nil
}

func inviteExpiresAt(expires string) time.Time {
	duration, err := time.ParseDuration(expires)
	if err != nil || duration <= 0 {
		duration = 10 * time.Minute
	}
	return time.Now().Add(duration)
}

func ParseSpokeOutput(resp string) []SpokeInfo {
	spokes := make([]SpokeInfo, 0)
	scanner := bufio.NewScanner(strings.NewReader(resp))
	var current SpokeInfo

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if current.ProtocolAddress != "" && current.Type != "local" {
				spokes = append(spokes, current)
			}
			current = SpokeInfo{}
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "interface", "iface":
			current.Interface = val
		case "type":
			current.Type = strings.ToLower(val)
		case "protocol-address", "protocol":
			current.ProtocolAddress = val
		case "nbma-address", "nbma":
			current.NBMAAddress = val
		case "nbma-nat-oa", "nbma-nat-oa-address", "nat-oa":
			current.NATAddress = val
		case "flags":
			current.Flags = val
			if strings.Contains(strings.ToLower(val), "shadow") {
				current.Type = "shadow"
			} else if strings.Contains(strings.ToLower(val), "static") {
				current.Type = "static"
			} else if current.Type == "" {
				current.Type = "direct"
			}
		case "expires-in", "expires", "holding-time":
			val = strings.TrimSpace(val)
			if strings.Contains(val, ":") {
				subparts := strings.Split(val, ":")
				if len(subparts) == 2 {
					m, _ := strconv.Atoi(subparts[0])
					s, _ := strconv.Atoi(subparts[1])
					current.ExpiresInSec = m*60 + s
					current.HoldingTime = m*60 + s
				} else if len(subparts) == 3 {
					h, _ := strconv.Atoi(subparts[0])
					m, _ := strconv.Atoi(subparts[1])
					s, _ := strconv.Atoi(subparts[2])
					current.ExpiresInSec = h*3600 + m*60 + s
					current.HoldingTime = current.ExpiresInSec
				}
			} else {
				sec, _ := strconv.Atoi(val)
				current.ExpiresInSec = sec
				current.HoldingTime = sec
			}
		}
	}
	if current.ProtocolAddress != "" && current.Type != "local" {
		spokes = append(spokes, current)
	}

	return spokes
}

func ParseInterfaceOutput(resp string) []InterfaceInfo {
	ifaces := make([]InterfaceInfo, 0)
	scanner := bufio.NewScanner(strings.NewReader(resp))
	var current InterfaceInfo

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			if current.Name != "" && (current.ProtocolAddress != "" || strings.Contains(current.Flags, "configured")) {
				ifaces = append(ifaces, current)
				current = InterfaceInfo{}
			}
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "interface", "iface":
			current.Name = val
		case "type":
			current.Type = val
		case "protocol-address":
			current.ProtocolAddress = val
		case "nbma-address":
			current.NBMAAddress = val
		case "mtu":
			mtu, _ := strconv.Atoi(val)
			current.MTU = mtu
		case "flags":
			current.Flags = val
		}
	}
	if current.Name != "" && (current.ProtocolAddress != "" || strings.Contains(current.Flags, "configured")) {
		ifaces = append(ifaces, current)
	}
	return ifaces
}

// ParseJSON parses a JSON string into target struct
func ParseJSON[T any](raw string) (*T, error) {
	raw = strings.TrimSpace(raw)
	// If response has multiple lines or prefix text, find start of JSON
	start := strings.Index(raw, "{")
	if start == -1 {
		start = strings.Index(raw, "[")
	}
	if start > 0 {
		raw = raw[start:]
	}

	var target T
	if err := json.Unmarshal([]byte(raw), &target); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w (raw: %s)", err, raw)
	}
	return &target, nil
}
