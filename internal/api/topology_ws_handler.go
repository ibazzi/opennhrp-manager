package api

import (
	"context"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/protocol"
	"opennhrp-manager/internal/service"
)

type TopologyWSHandler struct {
	nodeMgr    *service.NodeManager
	witnessSvc *service.WitnessService
	logHub     *service.LogHub
	spoke      *SpokeHandler
}

type liveMessage struct {
	Type     string             `json:"type"`
	Topology *topologySnapshot  `json:"topology,omitempty"`
	Log      *protocol.LogEntry `json:"log,omitempty"`
}

type topologySnapshot struct {
	NodeID        string                          `json:"node_id"`
	Nodes         []db.NodeRecord                 `json:"nodes"`
	Cluster       *executor.ClusterStatusInfo     `json:"cluster"`
	Replication   *executor.ReplicationStatusInfo `json:"replication,omitempty"`
	Invites       []executor.InviteRecord         `json:"invites,omitempty"`
	KeyStatus     *executor.KeyStatusInfo         `json:"key_status,omitempty"`
	Spokes        []executor.SpokeInfo            `json:"spokes"`
	SpokesByNode  map[string][]executor.SpokeInfo `json:"spokes_by_node"`
	SpokeFailures []string                        `json:"spoke_failures"`
	SLAMatrix     []service.NodeSLASummary        `json:"sla_matrix"`
	WitnessQuorum service.WitnessQuorumStatus     `json:"witness_quorum"`
	Timestamp     time.Time                       `json:"timestamp"`
}

func NewTopologyWSHandler(nodeMgr *service.NodeManager, witnessSvc *service.WitnessService, logHub *service.LogHub, spoke *SpokeHandler) *TopologyWSHandler {
	return &TopologyWSHandler{nodeMgr: nodeMgr, witnessSvc: witnessSvc, logHub: logHub, spoke: spoke}
}

func (h *TopologyWSHandler) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	nodeID := c.Query("node_id")
	includeHA := c.Query("include_ha") == "1"
	updates, unsubscribe := h.nodeMgr.SubscribeTopology()
	defer unsubscribe()
	if includeHA {
		unsubscribeHA := h.nodeMgr.SubscribeHA()
		defer unsubscribeHA()
	}
	logs, unsubscribeLogs := h.logHub.Subscribe()
	defer unsubscribeLogs()

	if err := h.writeSnapshot(c.Request.Context(), conn, nodeID, includeHA); err != nil {
		return
	}
	dirty := false
	flush := time.NewTicker(time.Second)
	ping := time.NewTicker(30 * time.Second)
	defer flush.Stop()
	defer ping.Stop()
	for {
		select {
		case _, ok := <-updates:
			if !ok {
				return
			}
			dirty = true
		case entry, ok := <-logs:
			if !ok || conn.WriteJSON(liveMessage{Type: "log", Log: &entry}) != nil {
				return
			}
		case <-flush.C:
			if dirty {
				dirty = false
				if err := h.writeSnapshot(context.Background(), conn, nodeID, includeHA); err != nil {
					return
				}
			}
		case <-ping.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)); err != nil {
				return
			}
		}
	}
}

func (h *TopologyWSHandler) writeSnapshot(ctx context.Context, conn *websocket.Conn, nodeID string, includeHA bool) error {
	if nodeID == "" {
		if exec, err := h.nodeMgr.GetHubExecutor(""); err == nil {
			nodeID = exec.GetNodeID()
		}
	}
	cluster, _ := h.nodeMgr.GetCachedClusterStatus(nodeID)
	var replication *executor.ReplicationStatusInfo
	var invites []executor.InviteRecord
	var keyStatus *executor.KeyStatusInfo
	if includeHA {
		replication, invites, keyStatus = h.nodeMgr.GetCachedHAStatus(nodeID)
	}
	spokesByNode := h.nodeMgr.GetCachedSpokesByNode()
	for id, spokes := range spokesByNode {
		if h.spoke != nil {
			h.spoke.decorateSpokes(spokes)
		}
		spokesByNode[id] = spokes
	}
	spokes := spokesByNode[nodeID]
	nodes, _ := h.nodeMgr.ListNodes(ctx)
	spokeFailures := make([]string, 0)
	for _, node := range nodes {
		if isHubNode(node) && node.Status != "offline" {
			if _, ok := spokesByNode[node.ID]; !ok {
				spokeFailures = append(spokeFailures, node.Name)
			}
		}
	}
	sort.Strings(spokeFailures)
	slaMatrix, _ := h.witnessSvc.GetSLAMatrix(ctx)
	snapshot := topologySnapshot{
		NodeID: nodeID, Nodes: nodes, Cluster: cluster, Replication: replication, Invites: invites, KeyStatus: keyStatus,
		Spokes: spokes, SpokesByNode: spokesByNode, SpokeFailures: spokeFailures,
		SLAMatrix: slaMatrix, WitnessQuorum: h.witnessSvc.GetQuorumStatus(), Timestamp: time.Now(),
	}
	return conn.WriteJSON(liveMessage{Type: "topology", Topology: &snapshot})
}
