package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/protocol"
)

type AgentClient struct {
	nodeID    string
	nodeType  string
	serverURL string
	token     string
	cfg       *config.Config

	sockCli *SocketClient

	wsConn  *websocket.Conn
	mu      sync.Mutex
	logChan chan protocol.LogEntry
}

func NewAgentClient(nodeID, nodeType, serverURL, token string, cfg *config.Config) *AgentClient {
	sockCli := NewSocketClient(cfg.OpenNHRPSocket, cfg.OpenNHRPHASocket, cfg.OpenNHRPCTLPath)

	return &AgentClient{
		nodeID:    nodeID,
		nodeType:  nodeType,
		serverURL: serverURL,
		token:     token,
		cfg:       cfg,
		sockCli:   sockCli,
		logChan:   make(chan protocol.LogEntry, 1000),
	}
}

func (a *AgentClient) Start(ctx context.Context) {
	log.Printf("[Agent %s] Starting opennhrp-agent, connecting to %s...", a.nodeID, a.serverURL)

	NewLogTailer(a.nodeID, a.logChan).Start(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := a.connectAndServe(ctx)
			if err != nil {
				log.Printf("[Agent %s] Connection lost (%v), reconnecting in 3s...", a.nodeID, err)
			}
			time.Sleep(3 * time.Second)
		}
	}
}

func (a *AgentClient) connectAndServe(ctx context.Context) error {
	u, err := url.Parse(a.serverURL)
	if err != nil {
		return fmt.Errorf("invalid server url: %w", err)
	}

	headers := http.Header{}
	headers.Set("X-Node-ID", a.nodeID)
	headers.Set("X-Node-Type", a.nodeType)
	headers.Set("X-Auth-Token", a.token)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), headers)
	if err != nil {
		return fmt.Errorf("websocket dial failed: %w", err)
	}
	defer conn.Close()

	a.mu.Lock()
	a.wsConn = conn
	a.mu.Unlock()

	log.Printf("[Agent %s] Successfully connected to Controller at %s", a.nodeID, u.Host)

	heartbeatTicker := time.NewTicker(500 * time.Millisecond)
	defer heartbeatTicker.Stop()

	done := make(chan struct{})

	// Goroutine for sending heartbeats and logs
	go func() {
		defer close(done)
		spokesCount := 0
		peerCount := 0
		coreAvailable := false
		var lastSpokeRefresh time.Time
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeatTicker.C:
				var status *executor.ClusterStatusInfo
				var clusterStatusJSON json.RawMessage
				if a.nodeType == "hub" {
					statusRaw, err := a.sockCli.SendSocketCommand(a.cfg.OpenNHRPHASocket, "ha cluster show --format json\n", 2*time.Second)
					if err == nil {
						status, _ = executor.ParseClusterStatusFromJSON(statusRaw, a.nodeID, a.cfg.StateDir)
						if status != nil {
							clusterStatusJSON, _ = json.Marshal(status)
						}
					}
				}

				var spokesJSON json.RawMessage
				var peersJSON json.RawMessage
				if time.Since(lastSpokeRefresh) >= 2*time.Second {
					lastSpokeRefresh = time.Now()
					spokesRaw, err := a.sockCli.SendSocketCommand(a.cfg.OpenNHRPSocket, "show\n", 2*time.Second)
					coreAvailable = err == nil
					if err == nil {
						spokes := executor.ParseSpokeOutput(spokesRaw)
						if a.nodeType == "spoke" {
							peerCount = len(spokes)
							peersJSON, _ = json.Marshal(spokes)
						} else {
							spokesCount = len(spokes)
							spokesJSON, _ = json.Marshal(spokes)
						}
					}
				}

				localRole := "unknown"
				var term uint64 = 0
				var commitIndex uint64 = 0
				networkHealth := false
				serviceAvail := false
				memberID := ""
				memberState := ""
				advertisedIP := ""
				clusterID := ""
				primary := ""
				leader := ""
				manifestRevision := uint64(0)
				digest := ""
				witness := protocol.WitnessPayload{}
				reportedSpokes := 0

				if status != nil {
					localRole = status.LocalRole
					term = status.Term
					commitIndex = status.CommitIndex
					networkHealth = status.NetworkHealth
					serviceAvail = status.ServiceAvail
					memberID = status.Member
					clusterID = status.ClusterID
					primary = status.Primary
					leader = status.Leader
					manifestRevision = status.ManifestRevision
					digest = status.Digest
					if status.LocalRole == "leader" {
						reportedSpokes = spokesCount
					}
					witness = protocol.WitnessPayload{
						Capable:             status.Witness.Capable,
						Mode:                status.Witness.Mode,
						Policy:              status.Witness.Policy,
						Voters:              status.Witness.Voters,
						Required:            status.Witness.Required,
						Votes:               status.Witness.Votes,
						Epoch:               status.Witness.Epoch,
						PeerVote:            status.Witness.PeerVote,
						ManagerVote:         status.Witness.ManagerVote,
						QuorumAvailable:     status.Witness.QuorumAvailable,
						LeaseHolder:         status.Witness.LeaseHolder,
						LeaseTerm:           status.Witness.LeaseTerm,
						LeaseSequence:       status.Witness.LeaseSequence,
						LeaseRemainingMS:    status.Witness.LeaseRemainingMS,
						FallbackRemainingMS: status.Witness.FallbackRemainingMS,
					}
					for _, mb := range status.Members {
						if mb.MemberID == status.Member {
							memberState = mb.State
						}
						if mb.MemberID == status.Member && len(mb.Advertised) > 0 {
							advertisedIP = mb.Advertised[0]
							break
						}
					}
				} else if a.nodeType == "spoke" {
					localRole = "spoke"
					networkHealth = coreAvailable && peerCount > 0
					serviceAvail = coreAvailable
				}

				payload := protocol.HeartbeatPayload{
					NodeID:           a.nodeID,
					NodeType:         a.nodeType,
					ClusterID:        clusterID,
					MemberID:         memberID,
					MemberState:      memberState,
					Primary:          primary,
					Leader:           leader,
					AdvertisedIP:     advertisedIP,
					LocalRole:        localRole,
					Term:             term,
					CommitIndex:      commitIndex,
					ManifestRevision: manifestRevision,
					Digest:           digest,
					NetworkHealth:    networkHealth,
					ServiceAvail:     serviceAvail,
					CoreAvailable:    coreAvailable,
					ActiveSpokes:     reportedSpokes,
					PeerCount:        peerCount,
					Timestamp:        time.Now(),
					Witness:          witness,
					ClusterStatus:    clusterStatusJSON,
					Spokes:           spokesJSON,
					Peers:            peersJSON,
				}

				envelope := protocol.Envelope{
					ID:        fmt.Sprintf("hb-%d", time.Now().UnixNano()),
					Type:      protocol.TypeHeartbeat,
					Timestamp: time.Now(),
					Payload:   payload,
				}

				a.mu.Lock()
				err = a.wsConn.WriteJSON(envelope)
				a.mu.Unlock()
				if err != nil {
					log.Printf("[Agent %s] Send heartbeat failed: %v", a.nodeID, err)
					return
				}

			case logEntry := <-a.logChan:
				envelope := protocol.Envelope{
					ID:        fmt.Sprintf("log-%d", time.Now().UnixNano()),
					Type:      protocol.TypeLogStream,
					Timestamp: time.Now(),
					Payload:   logEntry,
				}
				a.mu.Lock()
				err := a.wsConn.WriteJSON(envelope)
				a.mu.Unlock()
				if err != nil {
					return
				}
			}
		}
	}()

	// Read loop for commands from Manager
	for {
		var env protocol.Envelope
		err := conn.ReadJSON(&env)
		if err != nil {
			return fmt.Errorf("read json failed: %w", err)
		}

		if env.Type == protocol.TypeCommandRequest {
			go a.handleCommand(env)
		}
	}
}

func (a *AgentClient) handleCommand(env protocol.Envelope) {
	var req protocol.CommandRequest
	payloadBytes, _ := json.Marshal(env.Payload)
	_ = json.Unmarshal(payloadBytes, &req)

	resp := protocol.CommandResponse{Success: true}

	switch req.TargetSocket {
	case "opennhrp":
		out, err := a.sockCli.SendSocketCommand(a.cfg.OpenNHRPSocket, req.Command, 3*time.Second)
		if err != nil {
			resp.Success = false
			resp.Error = err.Error()
		} else {
			resp.RawText = out
		}
	case "opennhrp-ha":
		if a.nodeType != "hub" {
			resp.Success = false
			resp.Error = "HA socket is unavailable for spoke nodes"
			break
		}
		out, err := a.sockCli.SendSocketCommand(a.cfg.OpenNHRPHASocket, req.Command, 3*time.Second)
		if err != nil {
			resp.Success = false
			resp.Error = err.Error()
		} else if strings.HasPrefix(req.Command, "ha witness ") &&
			!strings.HasPrefix(out, "Status: ok\n") {
			resp.Success = false
			resp.Error = strings.TrimSpace(out)
		} else {
			resp.RawText = out
		}
	case "cli":
		out, err := a.sockCli.RunCLICommand(context.Background(), req.Args...)
		if err != nil {
			resp.Success = false
			resp.Error = err.Error()
		} else {
			resp.RawText = out
		}
	case "cli-stdin":
		cmd := exec.Command(a.cfg.OpenNHRPCTLPath, req.Args...)
		cmd.Stdin = strings.NewReader(req.Command + "\n")
		out, err := cmd.CombinedOutput()
		if err != nil {
			resp.Success = false
			resp.Error = fmt.Sprintf("%v: %s", err, string(out))
		} else {
			resp.RawText = string(out)
		}
	case "cli-export-key":
		tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("spoke-key-%d.keys", time.Now().UnixNano()))
		defer os.Remove(tmpFile)
		_, err := a.sockCli.RunCLICommand(context.Background(), "ha", "key", "export-spoke", tmpFile)
		if err != nil {
			resp.Success = false
			resp.Error = err.Error()
		} else {
			data, err := os.ReadFile(tmpFile)
			if err != nil {
				resp.Success = false
				resp.Error = err.Error()
			} else {
				resp.RawText = string(data)
			}
		}
	case "fs":
		if req.Command == "read-config" {
			data, err := os.ReadFile(a.cfg.ConfigPath)
			if err != nil {
				resp.Success = false
				resp.Error = err.Error()
			} else {
				resp.RawText = string(data)
			}
		} else if req.Command == "write-config" && len(req.Args) > 0 {
			err := writeFileAtomic(a.cfg.ConfigPath, []byte(req.Args[0]))
			if err != nil {
				resp.Success = false
				resp.Error = err.Error()
			}
		}
	default:
		resp.Success = false
		resp.Error = fmt.Sprintf("unknown target socket: %s", req.TargetSocket)
	}

	replyEnv := protocol.Envelope{
		ID:        env.ID,
		Type:      protocol.TypeCommandResponse,
		Timestamp: time.Now(),
		Payload:   resp,
	}

	a.mu.Lock()
	if a.wsConn != nil {
		_ = a.wsConn.WriteJSON(replyEnv)
	}
	a.mu.Unlock()
}

func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		old, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := atomicReplace(path+".bak", old, mode); err != nil {
			return fmt.Errorf("backup config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return atomicReplace(path, data, mode)
}

func atomicReplace(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	if handle, err := os.Open(dir); err == nil {
		_ = handle.Sync()
		_ = handle.Close()
	}
	return nil
}
