package executor

import (
	"context"
	"time"
)

type ClusterStatusInfo struct {
	Stale                bool               `json:"stale,omitempty"`
	Action               string             `json:"action,omitempty"`
	ClusterID            string             `json:"cluster_id,omitempty"`
	Primary              string             `json:"primary,omitempty"`
	Member               string             `json:"member"`
	Interface            string             `json:"interface,omitempty"`
	ProtocolAddress      string             `json:"protocol_address,omitempty"`
	PrefixLength         int                `json:"prefix_length,omitempty"`
	StateDir             string             `json:"state_dir,omitempty"`
	Term                 uint64             `json:"term"`
	CommitIndex          uint64             `json:"commit_index"`
	Leader               string             `json:"leader"`
	LocalRole            string             `json:"local_role"` // leader, standby, learner, isolated, witness
	ManifestRevision     uint64             `json:"manifest_revision"`
	Digest               string             `json:"digest"`
	ServiceAvail         bool               `json:"service_available"`
	NetworkHealth        bool               `json:"network_health"`
	NetworkHealthStatus  string             `json:"network_health_status"`
	HealthIntervalSec    int                `json:"health_interval_seconds"`
	HealthFailureRounds  int                `json:"health_failure_rounds"`
	HealthRecoveryRounds int                `json:"health_recovery_rounds"`
	Isolated             bool               `json:"isolated"`
	FailbackPending      bool               `json:"failback_pending,omitempty"`
	FailbackDeadline     string             `json:"failback_deadline,omitempty"`
	HealthTargets        []HealthTargetInfo `json:"health_targets,omitempty"`
	Members              []MemberInfo       `json:"members,omitempty"`
	Witness              WitnessStatusInfo  `json:"witness"`
}

type WitnessStatusInfo struct {
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
	Digest              string `json:"digest"`
}

type HealthTargetInfo struct {
	TargetIP      string  `json:"target_ip"`
	LastSuccess   bool    `json:"last_success"`
	ConsecutiveOk int     `json:"consecutive_ok,omitempty"`
	ConsecutiveNg int     `json:"consecutive_ng,omitempty"`
	IntervalSec   int     `json:"interval_sec"`
	RttMs         float64 `json:"rtt_ms,omitempty"`
}

type MemberInfo struct {
	MemberID      string   `json:"member_id"`
	Priority      int      `json:"priority"`
	State         string   `json:"state"` // active, learner, disabled
	IsLeader      bool     `json:"is_leader"`
	Advertised    []string `json:"advertised_addresses"`
	Observed      string   `json:"observed_address,omitempty"`
	MatchIndex    uint64   `json:"match_index,omitempty"`
	Lag           int64    `json:"lag,omitempty"`
	Connected     bool     `json:"connected"`
	Authenticated bool     `json:"authenticated"`
	RttMs         float64  `json:"rtt_ms,omitempty"`
	Digest        string   `json:"digest,omitempty"`
}

type ReplicationStatusInfo struct {
	LocalIndex        uint64                `json:"local_index"`
	Digest            string                `json:"digest"`
	SnapshotsSent     uint64                `json:"snapshots_sent"`
	DeltasSent        uint64                `json:"deltas_sent"`
	SnapshotsReceived uint64                `json:"snapshots_received"`
	DeltasReceived    uint64                `json:"deltas_received"`
	ResyncRequests    uint64                `json:"resync_requests"`
	Peers             []ReplicationPeerInfo `json:"peers"`
}

type ReplicationPeerInfo struct {
	MemberID   string `json:"member_id"`
	MatchIndex uint64 `json:"match_index"`
	Lag        int64  `json:"lag"`
	Digest     string `json:"digest"`
	Connected  bool   `json:"connected"`
}

type InviteParams struct {
	MemberID string `json:"member_id"`
	Priority int    `json:"priority"`
	Expires  string `json:"expires"` // e.g. "10m", "1h", "24h"
}

type InviteResult struct {
	MemberID    string    `json:"member_id"`
	InviteToken string    `json:"invite_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	Priority    int       `json:"priority"`
}

type InviteRecord struct {
	IDPrefix  string    `json:"id_prefix"`
	MemberID  string    `json:"member_id"`
	Priority  int       `json:"priority"`
	State     string    `json:"state"` // unused, claimed, expired, revoked
	ExpiresAt time.Time `json:"expires_at"`
}

type KeyStatusInfo struct {
	CurrentKeyID string `json:"current_key_id"`
	NextKeyID    string `json:"next_key_id,omitempty"`
	HasNextKey   bool   `json:"has_next_key"`
}

type SpokeInfo struct {
	Stale           bool       `json:"stale,omitempty"`
	ProtocolAddress string     `json:"protocol_address"`
	NBMAAddress     string     `json:"nbma_address"`
	NATAddress      string     `json:"nat_address,omitempty"`
	Interface       string     `json:"interface"`
	Type            string     `json:"type"` // direct, shadow, static, local
	Flags           string     `json:"flags"`
	HoldingTime     int        `json:"holding_time"`
	ExpiresInSec    int        `json:"expires_in_sec"`
	LastSeen        *time.Time `json:"last_seen,omitempty"`
	Alias           string     `json:"alias,omitempty"`
	SiteName        string     `json:"site_name,omitempty"`
	ManagedNodeID   string     `json:"managed_node_id,omitempty"`
	ManagedNodeName string     `json:"managed_node_name,omitempty"`
	ManagedStatus   string     `json:"managed_status,omitempty"`
}

type InterfaceInfo struct {
	Name            string `json:"name"`
	Type            string `json:"type,omitempty"`
	ProtocolAddress string `json:"protocol_address,omitempty"`
	MTU             int    `json:"mtu,omitempty"`
	NBMAMTU         int    `json:"nbma_mtu,omitempty"`
	NBMAAddress     string `json:"nbma_address,omitempty"`
	NATAddress      string `json:"nat_address,omitempty"`
	LinkName        string `json:"link_name,omitempty"`
	Flags           string `json:"flags,omitempty"`
	IsUp            bool   `json:"is_up,omitempty"`
	RxPackets       uint64 `json:"rx_packets,omitempty"`
	TxPackets       uint64 `json:"tx_packets,omitempty"`
}

type NodeExecutor interface {
	GetNodeID() string
	GetNodeType() string

	// OpenNHRP HA Cluster Management
	GetClusterStatus(ctx context.Context) (*ClusterStatusInfo, error)
	GetReplicationStatus(ctx context.Context) (*ReplicationStatusInfo, error)
	GetMembers(ctx context.Context) ([]MemberInfo, error)
	SetMember(ctx context.Context, memberID string, priority int, disabled *bool, remove bool) error

	// Invite Workflow
	CreateInvite(ctx context.Context, params InviteParams) (*InviteResult, error)
	ListInvites(ctx context.Context) ([]InviteRecord, error)
	RevokeInvite(ctx context.Context, idPrefix string) error
	DeleteInvite(ctx context.Context, idPrefix string) error
	JoinCluster(ctx context.Context, token, iface string, advertised []string) error

	// Key Rotation
	GetKeyStatus(ctx context.Context) (*KeyStatusInfo, error)
	RotateKey(ctx context.Context, action string) error // action: prepare | commit
	ExportSpokeKey(ctx context.Context) ([]byte, error)

	// Failback Control
	RequestFailback(ctx context.Context, force bool) error
	SendWitnessCommand(ctx context.Context, command string) error

	// Core Spoke / NHRP Cache Operations
	ListSpokes(ctx context.Context, iface string) ([]SpokeInfo, error)
	AddStaticMap(ctx context.Context, iface, protocolIP, nbmaIP string, register bool) error
	DelStaticMap(ctx context.Context, iface, protocolIP string) error
	SaveMap(ctx context.Context, iface string) error
	UpdateNBMA(ctx context.Context, protoIP, nbmaIP string) error
	PurgeRedirect(ctx context.Context, iface string) error

	// Interfaces & Config File
	GetInterfaces(ctx context.Context) ([]InterfaceInfo, error)
	ReadConfigFile(ctx context.Context) (string, error)
	WriteConfigFile(ctx context.Context, content string) error
	ReloadConfig(ctx context.Context) error
}
