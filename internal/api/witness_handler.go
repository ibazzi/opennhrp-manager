package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/service"
)

type WitnessHandler struct {
	witnessSvc *service.WitnessService
	database   *db.DB
}

func NewWitnessHandler(witnessSvc *service.WitnessService, database *db.DB) *WitnessHandler {
	return &WitnessHandler{witnessSvc: witnessSvc, database: database}
}

func (h *WitnessHandler) GetSLAMatrix(c *gin.Context) {
	matrix, err := h.witnessSvc.GetSLAMatrix(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, matrix)
}

func (h *WitnessHandler) GetQuorum(c *gin.Context) {
	c.JSON(http.StatusOK, h.witnessSvc.GetQuorumStatus())
}

func (h *WitnessHandler) GetRecentProbes(c *gin.Context) {
	nodeID := c.DefaultQuery("node_id", "")
	hours, _ := strconv.Atoi(c.DefaultQuery("hours", "24"))
	if hours <= 0 || hours > 24 {
		hours = 24
	}

	pointsText := c.Query("points")
	if pointsText == "" {
		pointsText = c.DefaultQuery("limit", "200")
	}
	points, _ := strconv.Atoi(pointsText)
	if points <= 0 || points > 500 {
		points = 200
	}

	probeType := c.DefaultQuery("probe_type", "all")
	switch probeType {
	case "all", "l3_nbma", "l4_port", "overlay_gre", "agent_telemetry":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid probe_type"})
		return
	}

	probes, err := h.database.GetProbes(nodeID, probeType, hours, points)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, probes)
}

func (h *WitnessHandler) GetArbitrations(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	arbitrations, err := h.database.GetArbitrations(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, arbitrations)
}
