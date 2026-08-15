package protocol

import (
	"encoding/json"
	"time"
)

// Message types between Manager and Agent
const (
	TypeRegisterRequest  = "register_request"
	TypeRegisterResponse = "register_response"
	TypeHeartbeat        = "heartbeat"
	TypeHeartbeatAck     = "heartbeat_ack"
	TypeCommandRequest   = "command_request"
	TypeCommandResponse  = "command_response"
	TypeLogStream        = "log_stream"
	TypeEventNotify      = "event_notify"
)

type Envelope struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

type RegisterRequest struct {
	NodeID       string   `json:"node_id"`
	Hostname     string   `json:"hostname"`
	Version      string   `json:"version"`
	AuthToken    string   `json:"auth_token"`
	AdvertisedIP []string `json:"advertised_ip"`
}

type RegisterResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type HeartbeatPayload struct {
	NodeID           string          `json:"node_id"`
	NodeType         string          `json:"node_type,omitempty"`
	ClusterID        string          `json:"cluster_id,omitempty"`
	MemberID         string          `json:"member_id,omitempty"`
	MemberState      string          `json:"member_state,omitempty"`
	Primary          string          `json:"primary,omitempty"`
	Leader           string          `json:"leader,omitempty"`
	AdvertisedIP     string          `json:"advertised_ip,omitempty"`
	LocalRole        string          `json:"local_role"` // leader, standby, learner, isolated
	Term             uint64          `json:"term"`
	CommitIndex      uint64          `json:"commit_index"`
	ManifestRevision uint64          `json:"manifest_revision"`
	Digest           string          `json:"digest,omitempty"`
	NetworkHealth    bool            `json:"network_health"`
	ServiceAvail     bool            `json:"service_available"`
	CoreAvailable    bool            `json:"core_available"`
	ActiveSpokes     int             `json:"active_spokes"`
	PeerCount        int             `json:"peer_count,omitempty"`
	UptimeSeconds    int64           `json:"uptime_seconds"`
	Timestamp        time.Time       `json:"timestamp"`
	Witness          WitnessPayload  `json:"witness"`
	ClusterStatus    json.RawMessage `json:"cluster_status,omitempty"`
	Spokes           json.RawMessage `json:"spokes,omitempty"`
	Peers            json.RawMessage `json:"peers,omitempty"`
}

type WitnessPayload struct {
	Capable             bool   `json:"capable"`
	Mode                string `json:"mode"`
	Policy              string `json:"policy"`
	Voters              int    `json:"voters"`
	Required            int    `json:"required"`
	Votes               int    `json:"votes"`
	Epoch               string `json:"epoch"`
	PeerVote            bool   `json:"peer_vote"`
	ManagerVote         bool   `json:"manager_vote"`
	QuorumAvailable     bool   `json:"quorum_available"`
	LeaseHolder         string `json:"lease_holder"`
	LeaseTerm           uint64 `json:"lease_term"`
	LeaseSequence       uint64 `json:"lease_sequence"`
	LeaseRemainingMS    uint64 `json:"lease_remaining_ms"`
	FallbackRemainingMS uint64 `json:"fallback_remaining_ms"`
}

type CommandRequest struct {
	TargetSocket string   `json:"target_socket"` // "opennhrp" or "opennhrp-ha" or "cli"
	Command      string   `json:"command"`
	Args         []string `json:"args,omitempty"`
}

type CommandResponse struct {
	Success  bool   `json:"success"`
	RawText  string `json:"raw_text,omitempty"`
	JSONData any    `json:"json_data,omitempty"`
	Error    string `json:"error,omitempty"`
}

type LogEntry struct {
	NodeID    string    `json:"node_id"`
	Source    string    `json:"source"` // "opennhrp", "opennhrp-ha", "system"
	Level     string    `json:"level"`  // "INFO", "WARN", "ERROR", "DEBUG"
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}
