package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/service"
)

var managedSpokeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type ManagedSpokeHandler struct {
	nodeMgr  *service.NodeManager
	database *db.DB
}

type managedSpokeView struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Status          string     `json:"status"`
	Host            string     `json:"host"`
	WSRttMs         float64    `json:"ws_rtt_ms"`
	CoreAvailable   bool       `json:"core_available"`
	PeerCount       int        `json:"peer_count"`
	LastSeen        *time.Time `json:"last_seen,omitempty"`
	ProtocolAddress string     `json:"protocol_address,omitempty"`
}

func normalizeManagedSpokeProtocolAddress(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", true
	}
	if ip, _, err := net.ParseCIDR(value); err == nil {
		return ip.String(), true
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String(), true
	}
	return "", false
}

func NewManagedSpokeHandler(nodeMgr *service.NodeManager, database *db.DB) *ManagedSpokeHandler {
	return &ManagedSpokeHandler{nodeMgr: nodeMgr, database: database}
}

func newManagedSpokeToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashManagedSpokeToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func managedSpokeTokenMatches(stored, token string) bool {
	want := hashManagedSpokeToken(token)
	return len(stored) == len(want) && subtle.ConstantTimeCompare([]byte(stored), []byte(want)) == 1
}

func (h *ManagedSpokeHandler) List(c *gin.Context) {
	nodes, err := h.nodeMgr.ListNodes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result := make([]managedSpokeView, 0)
	for _, node := range nodes {
		if node.Type != "spoke" {
			continue
		}
		view := managedSpokeView{
			ID: node.ID, Name: node.Name, Status: node.Status, Host: node.Host,
			WSRttMs: node.WSRttMs, CoreAvailable: node.ServiceAvail, ProtocolAddress: node.AdvertisedIP,
		}
		if !node.LastSeen.IsZero() {
			view.LastSeen = &node.LastSeen
		}
		if telemetry, ok := h.nodeMgr.GetNodeTelemetry(node.ID); ok {
			view.WSRttMs = telemetry.WSRttMs
			view.CoreAvailable = telemetry.CoreAvailable
			view.PeerCount = telemetry.PeerCount
		}
		result = append(result, view)
	}
	c.JSON(http.StatusOK, result)
}

func (h *ManagedSpokeHandler) Create(c *gin.Context) {
	var req struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		ProtocolAddress string `json:"protocol_address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.ID = strings.TrimSpace(req.ID)
	req.Name = strings.TrimSpace(req.Name)
	nameLen := utf8.RuneCountInString(req.Name)
	protocolAddress, validProtocolAddress := normalizeManagedSpokeProtocolAddress(req.ProtocolAddress)
	if !managedSpokeID.MatchString(req.ID) || nameLen == 0 || nameLen > 128 || !validProtocolAddress {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be 1-64 safe characters and name must be 1-128 characters"})
		return
	}
	if protocolAddress != "" {
		var existingID string
		err := h.database.QueryRow(`SELECT id FROM nodes WHERE type='spoke' AND advertised_ip=? LIMIT 1`, protocolAddress).Scan(&existingID)
		if err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "protocol address is already managed by " + existingID})
			return
		}
		if err != sql.ErrNoRows {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	token, err := newManagedSpokeToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	_, err = h.database.Exec(
		`INSERT INTO nodes (id, name, type, auth_token, status, role, advertised_ip, updated_at) VALUES (?, ?, 'spoke', ?, 'offline', 'spoke', ?, ?)`,
		req.ID, req.Name, hashManagedSpokeToken(token), protocolAddress, time.Now(),
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "node id already exists"})
		return
	}
	h.database.AddAuditLog(req.ID, c.GetString("username"), "create_managed_spoke", req.Name, true, "")
	c.JSON(http.StatusCreated, gin.H{"spoke": managedSpokeView{ID: req.ID, Name: req.Name, Status: "offline", ProtocolAddress: protocolAddress}, "token": token})
}

func (h *ManagedSpokeHandler) RotateToken(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		ProtocolAddress string `json:"protocol_address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	protocolAddress, validProtocolAddress := normalizeManagedSpokeProtocolAddress(req.ProtocolAddress)
	if !validProtocolAddress {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid protocol address"})
		return
	}
	if protocolAddress != "" {
		var existingID string
		err := h.database.QueryRow(`SELECT id FROM nodes WHERE type='spoke' AND advertised_ip=? AND id<>? LIMIT 1`, protocolAddress, id).Scan(&existingID)
		if err == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "protocol address is already managed by " + existingID})
			return
		}
		if err != sql.ErrNoRows {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	token, err := newManagedSpokeToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}
	query := `UPDATE nodes SET auth_token=?, updated_at=? WHERE id=? AND type='spoke'`
	args := []any{hashManagedSpokeToken(token), time.Now(), id}
	if protocolAddress != "" {
		query = `UPDATE nodes SET auth_token=?, advertised_ip=?, updated_at=? WHERE id=? AND type='spoke'`
		args = []any{hashManagedSpokeToken(token), protocolAddress, time.Now(), id}
	}
	result, err := h.database.Exec(query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "managed spoke not found"})
		return
	}
	h.nodeMgr.DisconnectAgent(id)
	h.database.AddAuditLog(id, c.GetString("username"), "rotate_managed_spoke_token", "", true, "")
	c.JSON(http.StatusOK, gin.H{"token": token})
}

func (h *ManagedSpokeHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	result, err := h.database.Exec(`DELETE FROM nodes WHERE id=? AND type='spoke'`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "managed spoke not found"})
		return
	}
	h.nodeMgr.DisconnectAgent(id)
	h.database.AddAuditLog(id, c.GetString("username"), "delete_managed_spoke", "", true, "")
	c.Status(http.StatusNoContent)
	c.Writer.WriteHeaderNow()
}

func (h *ManagedSpokeHandler) Peers(c *gin.Context) {
	id := c.Param("id")
	var nodeType string
	if err := h.database.QueryRow(`SELECT type FROM nodes WHERE id=?`, id).Scan(&nodeType); err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "managed spoke not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	if nodeType != "spoke" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node is not a managed spoke"})
		return
	}
	if exec, err := h.nodeMgr.GetExecutor(id); err == nil {
		peers, err := exec.ListSpokes(c.Request.Context(), "")
		if err == nil {
			h.nodeMgr.CacheSpokes(id, "", peers)
			c.JSON(http.StatusOK, peers)
			return
		}
	}
	peers, ok := h.nodeMgr.GetCachedSpokes(id, "")
	if !ok {
		peers = []executor.SpokeInfo{}
	}
	for i := range peers {
		peers[i].Stale = true
	}
	c.JSON(http.StatusOK, peers)
}
