package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/protocol"
	"opennhrp-manager/internal/service"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for local/agent connectivity
	},
}

type AgentWSHandler struct {
	cfg      *config.Config
	database *db.DB
	nodeMgr  *service.NodeManager
	logHub   *service.LogHub
}

func NewAgentWSHandler(cfg *config.Config, database *db.DB, nodeMgr *service.NodeManager, logHub *service.LogHub) *AgentWSHandler {
	return &AgentWSHandler{
		cfg:      cfg,
		database: database,
		nodeMgr:  nodeMgr,
		logHub:   logHub,
	}
}

func (h *AgentWSHandler) HandleWS(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		token = c.GetHeader("X-Auth-Token")
	}
	nodeID := c.Query("node_id")
	if nodeID == "" {
		nodeID = c.GetHeader("X-Node-ID")
	}
	nodeType := c.GetHeader("X-Node-Type")
	if nodeType == "" {
		nodeType = "hub"
	}

	if nodeID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id required in query or header"})
		return
	}
	if nodeType != "hub" && nodeType != "spoke" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node type must be hub or spoke"})
		return
	}
	if !h.authenticateAgent(nodeID, nodeType, token) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authentication token"})
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[AgentWS] Failed to upgrade websocket: %v", err)
		return
	}
	defer conn.Close()

	agentExec := h.nodeMgr.RegisterAgent(nodeID, nodeType, conn)
	defer h.nodeMgr.UnregisterAgent(nodeID, agentExec)

	log.Printf("[AgentWS] Agent connected: %s from %s", nodeID, conn.RemoteAddr())

	for {
		var env protocol.Envelope
		err := conn.ReadJSON(&env)
		if err != nil {
			log.Printf("[AgentWS] Read error from %s: %v", nodeID, err)
			break
		}

		switch env.Type {
		case protocol.TypeHeartbeat:
			var hb protocol.HeartbeatPayload
			if b, err := json.Marshal(env.Payload); err == nil {
				if err := json.Unmarshal(b, &hb); err == nil {
					hb.NodeType = nodeType
					h.nodeMgr.UpdateHeartbeat(nodeID, hb)
				}
			}

		case protocol.TypeCommandResponse:
			var resp protocol.CommandResponse
			if b, err := json.Marshal(env.Payload); err == nil {
				if err := json.Unmarshal(b, &resp); err == nil {
					agentExec.HandleResponse(env.ID, &resp)
				}
			}

		case protocol.TypeLogStream:
			var entry protocol.LogEntry
			if b, err := json.Marshal(env.Payload); err == nil {
				if err := json.Unmarshal(b, &entry); err == nil {
					entry.NodeID = nodeID
					h.logHub.Broadcast(entry)
				}
			}
		}
	}
}

func (h *AgentWSHandler) authenticateAgent(nodeID, nodeType, token string) bool {
	var storedType, storedToken string
	err := h.database.QueryRow(`SELECT type, auth_token FROM nodes WHERE id=?`, nodeID).Scan(&storedType, &storedToken)
	if nodeType == "spoke" {
		return err == nil && storedType == "spoke" && managedSpokeTokenMatches(storedToken, token)
	}
	if err == nil && storedType != "hub" && storedType != "agent" {
		return false
	}
	if err != nil && err != sql.ErrNoRows {
		return false
	}
	return h.cfg.AuthToken == "" || token == h.cfg.AuthToken
}
