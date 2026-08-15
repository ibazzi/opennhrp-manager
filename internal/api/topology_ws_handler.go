package api

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/service"
)

type TopologyWSHandler struct {
	nodeMgr    *service.NodeManager
	witnessSvc *service.WitnessService
}

type topologySnapshot struct {
	NodeID        string                      `json:"node_id"`
	Nodes         []db.NodeRecord             `json:"nodes"`
	Cluster       *executor.ClusterStatusInfo `json:"cluster"`
	Spokes        []executor.SpokeInfo        `json:"spokes"`
	SLAMatrix     []service.NodeSLASummary    `json:"sla_matrix"`
	WitnessQuorum service.WitnessQuorumStatus `json:"witness_quorum"`
	Timestamp     time.Time                   `json:"timestamp"`
}

func NewTopologyWSHandler(nodeMgr *service.NodeManager, witnessSvc *service.WitnessService) *TopologyWSHandler {
	return &TopologyWSHandler{nodeMgr: nodeMgr, witnessSvc: witnessSvc}
}

func (h *TopologyWSHandler) HandleWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	nodeID := c.Query("node_id")
	updates, unsubscribe := h.nodeMgr.SubscribeTopology()
	defer unsubscribe()

	if err := h.writeSnapshot(c.Request.Context(), conn, nodeID); err != nil {
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
		case <-flush.C:
			if dirty {
				dirty = false
				if err := h.writeSnapshot(context.Background(), conn, nodeID); err != nil {
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

func (h *TopologyWSHandler) writeSnapshot(ctx context.Context, conn *websocket.Conn, nodeID string) error {
	if nodeID == "" {
		if exec, err := h.nodeMgr.GetHubExecutor(""); err == nil {
			nodeID = exec.GetNodeID()
		}
	}
	cluster, _ := h.nodeMgr.GetCachedClusterStatus(nodeID)
	spokes, _ := h.nodeMgr.GetCachedSpokes(nodeID, "")
	nodes, _ := h.nodeMgr.ListNodes(ctx)
	slaMatrix, _ := h.witnessSvc.GetSLAMatrix(ctx)
	return conn.WriteJSON(topologySnapshot{
		NodeID: nodeID, Nodes: nodes, Cluster: cluster, Spokes: spokes,
		SLAMatrix: slaMatrix, WitnessQuorum: h.witnessSvc.GetQuorumStatus(), Timestamp: time.Now(),
	})
}
