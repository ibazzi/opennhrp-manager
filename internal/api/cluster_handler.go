package api

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/service"
)

type ClusterHandler struct {
	nodeMgr  *service.NodeManager
	database *db.DB
}

func NewClusterHandler(nodeMgr *service.NodeManager, database *db.DB) *ClusterHandler {
	return &ClusterHandler{nodeMgr: nodeMgr, database: database}
}

func memberCommandError(err error, memberID string) string {
	message := err.Error()
	if strings.Contains(message, "the current Leader must transfer leadership first") {
		return fmt.Sprintf("无法禁用节点 %s：该节点当前仍是 Leader。请先执行 Leader 回切或切换，待集群状态收敛后重试", memberID)
	}
	if strings.Contains(message, "only the current Leader can update membership") {
		return "成员变更只能由当前 Leader 执行。集群可能正在切换，请刷新状态后重试"
	}
	return message
}

func (h *ClusterHandler) GetClusterStatus(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		h.writeCachedClusterStatus(c, nodeID, err)
		return
	}

	status, err := exec.GetClusterStatus(c.Request.Context())
	if err != nil {
		h.writeCachedClusterStatus(c, exec.GetNodeID(), err)
		return
	}
	h.nodeMgr.CacheClusterStatus(exec.GetNodeID(), status)
	if nodeID != "" && nodeID != exec.GetNodeID() {
		h.nodeMgr.CacheClusterStatus(nodeID, status)
	}

	if nodeID != "" && status != nil && status.Member != "" {
		var advIP string
		for _, mb := range status.Members {
			if mb.MemberID == status.Member && len(mb.Advertised) > 0 {
				advIP = mb.Advertised[0]
				break
			}
		}
		_, _ = h.database.Exec(
			`UPDATE nodes SET name=?, role=?, term=?, advertised_ip=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			status.Member, status.LocalRole, status.Term, advIP, nodeID,
		)
	}

	c.JSON(http.StatusOK, status)
}

func (h *ClusterHandler) writeCachedClusterStatus(c *gin.Context, nodeID string, cause error) {
	status, ok := h.nodeMgr.GetCachedClusterStatus(nodeID)
	if !ok {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": cause.Error()})
		return
	}
	status.Stale = true
	c.JSON(http.StatusOK, status)
}

func (h *ClusterHandler) GetReplicationStatus(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusOK, &executor.ReplicationStatusInfo{Peers: []executor.ReplicationPeerInfo{}})
		return
	}

	status, err := exec.GetReplicationStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, &executor.ReplicationStatusInfo{Peers: []executor.ReplicationPeerInfo{}})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *ClusterHandler) GetMembers(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusOK, []executor.MemberInfo{})
		return
	}

	members, err := exec.GetMembers(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, []executor.MemberInfo{})
		return
	}
	sort.SliceStable(members, func(i, j int) bool {
		if members[i].Priority != members[j].Priority {
			return members[i].Priority > members[j].Priority
		}
		return members[i].MemberID < members[j].MemberID
	})
	c.JSON(http.StatusOK, members)
}

func (h *ClusterHandler) SetMember(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	var req struct {
		MemberID string `json:"member_id" binding:"required"`
		Priority int    `json:"priority"`
		Disabled *bool  `json:"disabled"`
		Remove   bool   `json:"remove"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Disabled != nil && *req.Disabled {
		status, err := exec.GetClusterStatus(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Unable to validate safe member disable: " + err.Error()})
			return
		}
		if status.Leader == req.MemberID {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("无法禁用节点 %s：该节点当前仍是 Leader。请先执行 Leader 回切或切换，待集群状态收敛后重试", req.MemberID)})
			return
		}
		if err := h.nodeMgr.ValidateMemberDisable(status, req.MemberID); err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
	}

	if err := exec.SetMember(c.Request.Context(), req.MemberID, req.Priority, req.Disabled, req.Remove); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": memberCommandError(err, req.MemberID)})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "set_member", req.MemberID, true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *ClusterHandler) CreateInvite(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	var req executor.InviteParams
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	res, err := exec.CreateInvite(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "create_invite", req.MemberID, true, "")
	c.JSON(http.StatusOK, res)
}

func (h *ClusterHandler) ListInvites(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusOK, []executor.InviteRecord{})
		return
	}

	invites, err := exec.ListInvites(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, []executor.InviteRecord{})
		return
	}
	c.JSON(http.StatusOK, invites)
}

func (h *ClusterHandler) RevokeInvite(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	idPrefix := c.Param("id_prefix")
	if err := exec.RevokeInvite(c.Request.Context(), idPrefix); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "revoke_invite", idPrefix, true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *ClusterHandler) DeleteInvite(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	idPrefix := c.Param("id_prefix")
	if err := exec.DeleteInvite(c.Request.Context(), idPrefix); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "delete_invite", idPrefix, true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *ClusterHandler) JoinCluster(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	var req struct {
		InviteToken string   `json:"invite_token" binding:"required"`
		Interface   string   `json:"interface" binding:"required"`
		Advertised  []string `json:"advertised_addresses"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := exec.JoinCluster(c.Request.Context(), req.InviteToken, req.Interface, req.Advertised); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "join_cluster", req.Interface, true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *ClusterHandler) GetKeyStatus(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusOK, &executor.KeyStatusInfo{})
		return
	}

	res, err := exec.GetKeyStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, &executor.KeyStatusInfo{})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *ClusterHandler) RotateKey(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	var req struct {
		Action string `json:"action" binding:"required"` // prepare or commit
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := exec.RotateKey(c.Request.Context(), req.Action); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "rotate_key_"+req.Action, "", true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *ClusterHandler) ExportSpokeKey(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	data, err := exec.ExportSpokeKey(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Disposition", "attachment; filename=gre-ha.keys")
	c.Data(http.StatusOK, "application/octet-stream", data)
}

func (h *ClusterHandler) RequestFailback(c *gin.Context) {
	nodeID := c.Query("node_id")
	exec, err := h.nodeMgr.GetHubExecutor(nodeID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "No opennhrp-agent connected: " + err.Error()})
		return
	}

	force, _ := strconv.ParseBool(c.DefaultQuery("force", "false"))
	if err := exec.RequestFailback(c.Request.Context(), force); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.database.AddAuditLog(nodeID, "admin", "request_failback", fmt.Sprintf("force=%v", force), true, "")
	c.JSON(http.StatusOK, gin.H{"success": true})
}
