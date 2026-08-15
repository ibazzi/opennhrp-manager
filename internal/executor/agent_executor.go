package executor

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"opennhrp-manager/internal/protocol"
)

type AgentExecutor struct {
	nodeID    string
	nodeType  string
	wsConn    *websocket.Conn
	mu        sync.Mutex
	pendingMu sync.Mutex
	pending   map[string]chan *protocol.CommandResponse
}

type AgentCommandError struct {
	Detail string
}

func (e *AgentCommandError) Error() string {
	return "agent command error: " + e.Detail
}

func NewAgentExecutor(nodeID, nodeType string, wsConn *websocket.Conn) *AgentExecutor {
	return &AgentExecutor{
		nodeID:   nodeID,
		nodeType: nodeType,
		wsConn:   wsConn,
		pending:  make(map[string]chan *protocol.CommandResponse),
	}
}

func (a *AgentExecutor) GetNodeID() string {
	return a.nodeID
}

func (a *AgentExecutor) GetNodeType() string {
	return a.nodeType
}

func (a *AgentExecutor) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.wsConn == nil {
		return nil
	}
	err := a.wsConn.Close()
	a.wsConn = nil
	return err
}

func (a *AgentExecutor) UpdateConn(conn *websocket.Conn) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.wsConn = conn
}

// HandleResponse routes incoming response envelope to waiting caller
func (a *AgentExecutor) HandleResponse(msgID string, resp *protocol.CommandResponse) {
	a.pendingMu.Lock()
	ch, exists := a.pending[msgID]
	a.pendingMu.Unlock()

	if exists && ch != nil {
		select {
		case ch <- resp:
		default:
		}
	}
}

func (a *AgentExecutor) sendCommand(ctx context.Context, targetSocket, command string, args []string) (*protocol.CommandResponse, error) {
	a.mu.Lock()
	conn := a.wsConn
	a.mu.Unlock()

	if conn == nil {
		return nil, fmt.Errorf("agent %s is offline", a.nodeID)
	}

	reqID := uuid.New().String()
	respChan := make(chan *protocol.CommandResponse, 1)

	a.pendingMu.Lock()
	a.pending[reqID] = respChan
	a.pendingMu.Unlock()

	defer func() {
		a.pendingMu.Lock()
		delete(a.pending, reqID)
		a.pendingMu.Unlock()
	}()

	envelope := protocol.Envelope{
		ID:        reqID,
		Type:      protocol.TypeCommandRequest,
		Timestamp: time.Now(),
		Payload: protocol.CommandRequest{
			TargetSocket: targetSocket,
			Command:      command,
			Args:         args,
		},
	}

	a.mu.Lock()
	err := conn.WriteJSON(envelope)
	a.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("send to agent failed: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("agent response timeout")
	case resp := <-respChan:
		if !resp.Success {
			return nil, &AgentCommandError{Detail: resp.Error}
		}
		return resp, nil
	}
}

func (a *AgentExecutor) GetClusterStatus(ctx context.Context) (*ClusterStatusInfo, error) {
	resp, err := a.sendCommand(ctx, "opennhrp-ha", "ha cluster show --format json\n", nil)
	if err != nil {
		return nil, err
	}
	return ParseClusterStatusFromJSON(resp.RawText, a.nodeID, "")
}

func (a *AgentExecutor) GetReplicationStatus(ctx context.Context) (*ReplicationStatusInfo, error) {
	resp, err := a.sendCommand(ctx, "opennhrp-ha", "ha replication show --format json\n", nil)
	if err != nil {
		return nil, err
	}
	return ParseReplicationStatusFromJSON(resp.RawText)
}

func (a *AgentExecutor) GetMembers(ctx context.Context) ([]MemberInfo, error) {
	status, err := a.GetClusterStatus(ctx)
	if err != nil {
		return nil, err
	}
	return status.Members, nil
}

func (a *AgentExecutor) SetMember(ctx context.Context, memberID string, priority int, disabled *bool, remove bool) error {
	var args []string
	if remove {
		args = []string{"ha", "member", "remove", memberID}
	} else if disabled != nil {
		action := "enable"
		if *disabled {
			action = "disable"
		}
		args = []string{"ha", "member", action, memberID}
	} else {
		args = []string{"ha", "member", "set", memberID, "--priority", strconv.Itoa(priority)}
	}
	_, err := a.sendCommand(ctx, "cli", "", args)
	return err
}

func (a *AgentExecutor) CreateInvite(ctx context.Context, params InviteParams) (*InviteResult, error) {
	args := []string{"ha", "invite", "create", "--member-id", params.MemberID, "--format", "plain"}
	if params.Priority > 0 {
		args = append(args, "--priority", strconv.Itoa(params.Priority))
	}
	if params.Expires != "" {
		args = append(args, "--expires", params.Expires)
	}

	resp, err := a.sendCommand(ctx, "cli", "", args)
	if err != nil {
		return nil, fmt.Errorf("create invite failed: %w", err)
	}

	return &InviteResult{
		MemberID:    params.MemberID,
		InviteToken: strings.TrimSpace(resp.RawText),
		Priority:    params.Priority,
		ExpiresAt:   inviteExpiresAt(params.Expires),
	}, nil
}

func (a *AgentExecutor) ListInvites(ctx context.Context) ([]InviteRecord, error) {
	resp, err := a.sendCommand(ctx, "cli", "", []string{"ha", "invite", "list", "--format", "json"})
	if err != nil {
		return nil, err
	}
	return ParseInviteListFromJSON(resp.RawText)
}

func (a *AgentExecutor) RevokeInvite(ctx context.Context, idPrefix string) error {
	_, err := a.sendCommand(ctx, "cli", "", []string{"ha", "invite", "revoke", "--id-prefix", idPrefix})
	return err
}

func (a *AgentExecutor) DeleteInvite(ctx context.Context, idPrefix string) error {
	_, err := a.sendCommand(ctx, "cli", "", []string{"ha", "invite", "delete", "--id-prefix", idPrefix})
	return err
}

func (a *AgentExecutor) JoinCluster(ctx context.Context, token, iface string, advertised []string) error {
	args := []string{"ha", "join", "--interface", iface}
	for _, addr := range advertised {
		args = append(args, "--advertise-address", addr)
	}
	_, err := a.sendCommand(ctx, "cli-stdin", token, args)
	return err
}

func (a *AgentExecutor) GetKeyStatus(ctx context.Context) (*KeyStatusInfo, error) {
	resp, err := a.sendCommand(ctx, "opennhrp-ha", "ha key status --format json\n", nil)
	if err != nil {
		return nil, err
	}
	if !strings.Contains(resp.RawText, "{") {
		return nil, fmt.Errorf("key status returned no JSON")
	}
	return ParseKeyStatusFromJSON(resp.RawText)
}

func (a *AgentExecutor) RotateKey(ctx context.Context, action string) error {
	if action != "prepare" && action != "commit" {
		return fmt.Errorf("invalid action %s; must be prepare or commit", action)
	}
	_, err := a.sendCommand(ctx, "cli", "", []string{"ha", "key", "rotate", action})
	return err
}

func (a *AgentExecutor) ExportSpokeKey(ctx context.Context) ([]byte, error) {
	resp, err := a.sendCommand(ctx, "cli-export-key", "", nil)
	if err != nil {
		return nil, err
	}
	return []byte(resp.RawText), nil
}

func (a *AgentExecutor) RequestFailback(ctx context.Context, force bool) error {
	cmd := "ha failback request\n"
	if force {
		cmd = "ha failback request force\n"
	}
	_, err := a.sendCommand(ctx, "opennhrp-ha", cmd, nil)
	return err
}

func (a *AgentExecutor) SendWitnessCommand(ctx context.Context, command string) error {
	_, err := a.sendCommand(ctx, "opennhrp-ha", command+"\n", nil)
	return err
}

func (a *AgentExecutor) ListSpokes(ctx context.Context, iface string) ([]SpokeInfo, error) {
	cmd := "show\n"
	if iface != "" {
		cmd = fmt.Sprintf("show interface %s\n", iface)
	}
	resp, err := a.sendCommand(ctx, "opennhrp", cmd, nil)
	if err != nil {
		return nil, err
	}
	return ParseSpokeOutput(resp.RawText), nil
}

func (a *AgentExecutor) AddStaticMap(ctx context.Context, iface, protocolIP, nbmaIP string, register bool) error {
	cmd := fmt.Sprintf("map add interface %s protocol %s nbma %s", iface, protocolIP, nbmaIP)
	if register {
		cmd += " register"
	}
	cmd += "\n"
	_, err := a.sendCommand(ctx, "opennhrp", cmd, nil)
	return err
}

func (a *AgentExecutor) DelStaticMap(ctx context.Context, iface, protocolIP string) error {
	cmd := fmt.Sprintf("map del interface %s protocol %s\n", iface, protocolIP)
	_, err := a.sendCommand(ctx, "opennhrp", cmd, nil)
	return err
}

func (a *AgentExecutor) SaveMap(ctx context.Context, iface string) error {
	cmd := fmt.Sprintf("map save interface %s\n", iface)
	_, err := a.sendCommand(ctx, "opennhrp", cmd, nil)
	return err
}

func (a *AgentExecutor) UpdateNBMA(ctx context.Context, protoIP, nbmaIP string) error {
	cmd := fmt.Sprintf("update nbma %s %s\n", protoIP, nbmaIP)
	_, err := a.sendCommand(ctx, "opennhrp", cmd, nil)
	return err
}

func (a *AgentExecutor) PurgeRedirect(ctx context.Context, iface string) error {
	cmd := "redirect purge"
	if iface != "" {
		cmd = fmt.Sprintf("redirect purge interface %s\n", iface)
	}
	cmd += "\n"
	_, err := a.sendCommand(ctx, "opennhrp", cmd, nil)
	return err
}

func (a *AgentExecutor) GetInterfaces(ctx context.Context) ([]InterfaceInfo, error) {
	resp, err := a.sendCommand(ctx, "opennhrp", "interface show\n", nil)
	if err != nil {
		return nil, err
	}
	return ParseInterfaceOutput(resp.RawText), nil
}

func (a *AgentExecutor) ReadConfigFile(ctx context.Context) (string, error) {
	resp, err := a.sendCommand(ctx, "fs", "read-config", nil)
	if err != nil {
		return "", err
	}
	return resp.RawText, nil
}

func (a *AgentExecutor) WriteConfigFile(ctx context.Context, content string) error {
	_, err := a.sendCommand(ctx, "fs", "write-config", []string{content})
	return err
}

func (a *AgentExecutor) ReloadConfig(ctx context.Context) error {
	if a.nodeType == "hub" {
		_, _ = a.sendCommand(ctx, "opennhrp-ha", "ha managed reload\n", nil)
	}
	resp, err := a.sendCommand(ctx, "opennhrp", "reload\n", nil)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp.RawText, "Status: ok\n") {
		return fmt.Errorf("reload rejected: %s", strings.TrimSpace(resp.RawText))
	}
	return nil
}
