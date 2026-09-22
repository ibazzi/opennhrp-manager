package api

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/service"
)

type SpokeHandler struct {
	nodeMgr  *service.NodeManager
	database *db.DB
}

func NewSpokeHandler(nodeMgr *service.NodeManager, database *db.DB) *SpokeHandler {
	return &SpokeHandler{
		nodeMgr:  nodeMgr,
		database: database,
	}
}

func (h *SpokeHandler) attachManagedSpokes(ctx context.Context, spokes []executor.SpokeInfo) {
	rows, err := h.database.QueryContext(ctx, `SELECT id, name, status, advertised_ip FROM nodes WHERE type='spoke' AND advertised_ip<>''`)
	if err != nil {
		return
	}
	defer rows.Close()
	type managedRef struct{ ID, Name, Status string }
	managed := make(map[string]managedRef)
	for rows.Next() {
		var id, name, status, address string
		if rows.Scan(&id, &name, &status, &address) == nil {
			if address, ok := normalizeManagedSpokeProtocolAddress(address); ok {
				managed[address] = managedRef{id, name, status}
			}
		}
	}
	for i := range spokes {
		spokes[i].ManagedNodeID, spokes[i].ManagedNodeName, spokes[i].ManagedStatus = "", "", ""
		address, ok := normalizeManagedSpokeProtocolAddress(spokes[i].ProtocolAddress)
		if ref, exists := managed[address]; ok && exists {
			spokes[i].ManagedNodeID, spokes[i].ManagedNodeName, spokes[i].ManagedStatus = ref.ID, ref.Name, ref.Status
		}
	}
}

func (h *SpokeHandler) ListSpokes(c *gin.Context) {
	nodeID := c.Query("node_id")
	iface := c.DefaultQuery("interface", "")

	spokes, err := h.listSpokes(c.Request.Context(), nodeID, iface)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, spokes)
}

func (h *SpokeHandler) listSpokes(ctx context.Context, nodeID, iface string) ([]executor.SpokeInfo, error) {
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		if spokes, ok := h.cachedSpokes(ctx, nodeID, iface); ok {
			return spokes, nil
		}
		return nil, err
	}

	spokes, err := exec.ListSpokes(ctx, iface)
	if err != nil {
		if cached, ok := h.cachedSpokes(ctx, exec.GetNodeID(), iface); ok {
			return cached, nil
		}
		return nil, err
	}

	h.decorateSpokes(ctx, spokes)
	h.nodeMgr.CacheSpokes(exec.GetNodeID(), iface, spokes)
	if nodeID != "" && nodeID != exec.GetNodeID() {
		h.nodeMgr.CacheSpokes(nodeID, iface, spokes)
	}
	return spokes, nil
}

func (h *SpokeHandler) decorateSpokes(ctx context.Context, spokes []executor.SpokeInfo) {
	rows, _ := h.database.QueryContext(ctx, "SELECT protocol_address, alias, site_name, notes FROM spoke_metadata")
	if rows != nil {
		metaMap := make(map[string]struct{ Alias, Site, Notes string })
		for rows.Next() {
			var ip, alias, site, notes string
			_ = rows.Scan(&ip, &alias, &site, &notes)
			metaMap[ip] = struct{ Alias, Site, Notes string }{Alias: alias, Site: site, Notes: notes}
		}
		for i := range spokes {
			if meta, exists := metaMap[spokes[i].ProtocolAddress]; exists {
				spokes[i].Alias = meta.Alias
				spokes[i].Notes = meta.Notes
				spokes[i].SiteName = meta.Site
			}
		}
		_ = rows.Close()
	}

	sort.Slice(spokes, func(i, j int) bool {
		ipA := net.ParseIP(strings.Split(spokes[i].ProtocolAddress, "/")[0])
		ipB := net.ParseIP(strings.Split(spokes[j].ProtocolAddress, "/")[0])
		if ipA != nil && ipB != nil {
			return bytes.Compare(ipA.To16(), ipB.To16()) < 0
		}
		return spokes[i].ProtocolAddress < spokes[j].ProtocolAddress
	})
	h.attachManagedSpokes(ctx, spokes)
}

func (h *SpokeHandler) cachedSpokes(ctx context.Context, nodeID, iface string) ([]executor.SpokeInfo, bool) {
	spokes, ok := h.nodeMgr.GetCachedSpokes(nodeID, iface)
	if !ok {
		return nil, false
	}
	for i := range spokes {
		spokes[i].Stale = true
	}
	h.decorateSpokes(ctx, spokes)
	return spokes, true
}

func isHubNode(node db.NodeRecord) bool {
	return node.ID != "local" && node.Type != "local" && node.Type != "spoke" && node.Role != "witness"
}

func (h *SpokeHandler) AddStaticMap(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetOpenNHRPExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	var req struct {
		Interface       string `json:"interface" binding:"required"`
		ProtocolAddress string `json:"protocol_address" binding:"required"`
		NBMAAddress     string `json:"nbma_address" binding:"required"`
		Register        bool   `json:"register"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := exec.AddStaticMap(c.Request.Context(), req.Interface, req.ProtocolAddress, req.NBMAAddress, req.Register); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "add_static_map", fmt.Sprintf("%s -> %s", req.ProtocolAddress, req.NBMAAddress), true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *SpokeHandler) DelStaticMap(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetOpenNHRPExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	var req struct {
		Interface       string `json:"interface" binding:"required"`
		ProtocolAddress string `json:"protocol_address" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := exec.DelStaticMap(c.Request.Context(), req.Interface, req.ProtocolAddress); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "del_static_map", req.ProtocolAddress, true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *SpokeHandler) SaveMap(c *gin.Context) {
	nodeID := c.Query("node_id")
	iface := c.DefaultQuery("interface", "gre-ha")
	exec, err := h.nodeMgr.GetOpenNHRPExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	if err := exec.SaveMap(c.Request.Context(), iface); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "save_map", iface, true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *SpokeHandler) UpdateNBMA(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetOpenNHRPExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	var req struct {
		ProtocolAddress string `json:"protocol_address" binding:"required"`
		NBMAAddress     string `json:"nbma_address" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := exec.UpdateNBMA(c.Request.Context(), req.ProtocolAddress, req.NBMAAddress); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "update_nbma", fmt.Sprintf("%s %s", req.ProtocolAddress, req.NBMAAddress), true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *SpokeHandler) PurgeRedirect(c *gin.Context) {
	nodeID := c.Query("node_id")
	protoIP := c.Query("protocol_address")
	exec, err := h.nodeMgr.GetOpenNHRPExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	if err := exec.PurgeRedirect(c.Request.Context(), protoIP); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "purge_redirect", protoIP, true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *SpokeHandler) SetSpokeMetadata(c *gin.Context) {
	var req db.SpokeMetaRecord
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.database.Exec(
		`INSERT INTO spoke_metadata (protocol_address, alias, site_name, contact, notes, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(protocol_address) DO UPDATE SET 
		 alias=excluded.alias, site_name=excluded.site_name, contact=excluded.contact, notes=excluded.notes, updated_at=excluded.updated_at`,
		req.ProtocolAddress, req.Alias, req.SiteName, req.Contact, req.Notes, time.Now(),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.nodeMgr != nil {
		h.nodeMgr.NotifyTopology()
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
