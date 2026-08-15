package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/service"
)

type ConfigHandler struct {
	nodeMgr  *service.NodeManager
	database *db.DB
}

func NewConfigHandler(nodeMgr *service.NodeManager, database *db.DB) *ConfigHandler {
	return &ConfigHandler{nodeMgr: nodeMgr, database: database}
}

func (h *ConfigHandler) ListNodes(c *gin.Context) {
	nodes, err := h.nodeMgr.ListNodes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, nodes)
}

func (h *ConfigHandler) ListInterfaces(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}

	ifaces, err := exec.GetInterfaces(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, []interface{}{})
		return
	}
	c.JSON(http.StatusOK, ifaces)
}

func (h *ConfigHandler) GetConfigFile(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"content": "# No opennhrp-agent connected. Connect an opennhrp-agent to view and edit opennhrp.conf."})
		return
	}

	content, err := exec.ReadConfigFile(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"content": content})
}

func (h *ConfigHandler) SaveConfigFile(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := exec.WriteConfigFile(c.Request.Context(), req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Record config history
	_, _ = h.database.Exec(
		"INSERT INTO config_history (node_id, content, comment, created_at) VALUES (?, ?, ?, ?)",
		nodeID, req.Content, req.Comment, time.Now(),
	)

	h.database.AddAuditLog(nodeID, "admin", "save_config", req.Comment, true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *ConfigHandler) ReloadConfig(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	if err := exec.ReloadConfig(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "reload_config", "", true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *ConfigHandler) GetAuditLogs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	logs, total, err := h.database.GetAuditLogs(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"total": total,
		"items": logs,
	})
}
