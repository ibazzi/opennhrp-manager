package api

import (
	"bytes"
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
	nodeMgr     *service.NodeManager
	database    *db.DB
	provService *service.ProvisioningService
}

func NewSpokeHandler(nodeMgr *service.NodeManager, database *db.DB, provService *service.ProvisioningService) *SpokeHandler {
	return &SpokeHandler{
		nodeMgr:     nodeMgr,
		database:    database,
		provService: provService,
	}
}

func (h *SpokeHandler) attachManagedSpokes(spokes []executor.SpokeInfo) {
	rows, err := h.database.Query(`SELECT id, name, status, advertised_ip FROM nodes WHERE type='spoke' AND advertised_ip<>''`)
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

	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		h.writeCachedSpokes(c, nodeID, iface, err)
		return
	}

	spokes, err := exec.ListSpokes(c.Request.Context(), iface)
	if err != nil {
		h.writeCachedSpokes(c, exec.GetNodeID(), iface, err)
		return
	}

	// Attach metadata from DB if available
	rows, _ := h.database.Query("SELECT protocol_address, alias, site_name FROM spoke_metadata")
	if rows != nil {
		metaMap := make(map[string]struct{ Alias, Site string })
		for rows.Next() {
			var ip, alias, site string
			_ = rows.Scan(&ip, &alias, &site)
			metaMap[ip] = struct{ Alias, Site string }{Alias: alias, Site: site}
		}
		for i := range spokes {
			if meta, exists := metaMap[spokes[i].ProtocolAddress]; exists {
				spokes[i].Alias = meta.Alias
				spokes[i].SiteName = meta.Site
			}
		}
		_ = rows.Close()
	}

	// Sort spokes by protocol IP address ascending
	sort.Slice(spokes, func(i, j int) bool {
		ipA := net.ParseIP(strings.Split(spokes[i].ProtocolAddress, "/")[0])
		ipB := net.ParseIP(strings.Split(spokes[j].ProtocolAddress, "/")[0])
		if ipA != nil && ipB != nil {
			return bytes.Compare(ipA.To16(), ipB.To16()) < 0
		}
		return spokes[i].ProtocolAddress < spokes[j].ProtocolAddress
	})
	h.nodeMgr.CacheSpokes(exec.GetNodeID(), iface, spokes)
	if nodeID != "" && nodeID != exec.GetNodeID() {
		h.nodeMgr.CacheSpokes(nodeID, iface, spokes)
	}

	h.attachManagedSpokes(spokes)
	c.JSON(http.StatusOK, spokes)
}

func (h *SpokeHandler) writeCachedSpokes(c *gin.Context, nodeID, iface string, cause error) {
	spokes, ok := h.nodeMgr.GetCachedSpokes(nodeID, iface)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": cause.Error()})
		return
	}
	for i := range spokes {
		spokes[i].Stale = true
	}
	h.attachManagedSpokes(spokes)
	c.JSON(http.StatusOK, spokes)
}

func (h *SpokeHandler) AddStaticMap(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
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
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
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
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
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
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
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
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
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

func (h *SpokeHandler) GenerateSpokeConfig(c *gin.Context) {
	var req service.SpokeConfigTemplateParams
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	conf := h.provService.GenerateOpenNHRPConf(req)
	script := h.provService.GenerateSetupScript(req)

	c.JSON(http.StatusOK, gin.H{
		"opennhrp_conf": conf,
		"setup_script":  script,
	})
}

func (h *SpokeHandler) DownloadSpokePackage(c *gin.Context) {
	var req service.SpokeConfigTemplateParams
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	nodeID := c.Query("node_id")
	exec, _ := h.nodeMgr.GetHubExecutor(nodeID)
	var keyBytes []byte
	if exec != nil {
		keyBytes, _ = exec.ExportSpokeKey(c.Request.Context())
	}

	zipData, err := h.provService.BuildPackageZip(req, keyBytes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Disposition", "attachment; filename=opennhrp-spoke-package.zip")
	c.Data(http.StatusOK, "application/zip", zipData)
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
	c.JSON(http.StatusOK, gin.H{"success": true})
}
