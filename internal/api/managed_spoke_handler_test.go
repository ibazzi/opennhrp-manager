package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/service"
)

func managedSpokeTestContext(method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("username", "tester")
	return ctx, recorder
}

func TestManagedSpokeCreateValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := db.InitDB(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	handler := NewManagedSpokeHandler(nil, database)
	for _, tc := range []struct {
		name, id, displayName, address, wantError string
	}{
		{"missing name", "ctyun", "", "", "显示名称"},
		{"blank name", "ctyun", "  ", "", "显示名称"},
		{"long name", "ctyun", strings.Repeat("中", 129), "", "显示名称"},
		{"invalid id", "bad id", "上海分支 01", "", "节点 ID"},
		{"invalid IP", "ctyun", "上海分支 01", "bad-ip", "Protocol IP"},
		{"Chinese name without IP", " ctyun ", " 上海分支 01 ", "", ""},
		{"maximum name", "branch-2", strings.Repeat("中", 128), "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]string{"id": tc.id, "name": tc.displayName, "protocol_address": tc.address})
			if err != nil {
				t.Fatal(err)
			}
			ctx, recorder := managedSpokeTestContext(http.MethodPost, "/api/managed-spokes", string(body))
			handler.Create(ctx)
			if tc.wantError != "" {
				if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), tc.wantError) {
					t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
				}
				return
			}
			if recorder.Code != http.StatusCreated {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			var name string
			if err := database.QueryRow(`SELECT name FROM nodes WHERE id=?`, strings.TrimSpace(tc.id)).Scan(&name); err != nil || name != strings.TrimSpace(tc.displayName) {
				t.Fatalf("stored name=%q err=%v", name, err)
			}
		})
	}
}

func TestManagedSpokeTokenLifecycleAndAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	database, err := db.InitDB(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	nodeMgr := service.NewNodeManager(&config.Config{}, database, service.NewLogHub())
	handler := NewManagedSpokeHandler(nodeMgr, database)

	ctx, recorder := managedSpokeTestContext(http.MethodPost, "/api/managed-spokes", `{"id":"branch-1","name":"Branch 1","protocol_address":"10.20.0.2/32"}`)
	handler.Create(ctx)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var created struct {
		Token string `json:"token"`
		Spoke struct {
			ProtocolAddress string `json:"protocol_address"`
		} `json:"spoke"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil || created.Token == "" {
		t.Fatalf("missing one-time token: %v body=%s", err, recorder.Body.String())
	}
	if created.Spoke.ProtocolAddress != "10.20.0.2" {
		t.Fatalf("protocol binding was not normalized: %#v", created.Spoke)
	}
	var stored string
	if err := database.QueryRow(`SELECT auth_token FROM nodes WHERE id='branch-1'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == created.Token || !strings.HasPrefix(stored, "sha256:") {
		t.Fatalf("token was not hashed: %q", stored)
	}
	observed := []executor.SpokeInfo{{ProtocolAddress: "10.20.0.2/24"}}
	NewSpokeHandler(nodeMgr, database, nil).attachManagedSpokes(observed)
	if observed[0].ManagedNodeID != "branch-1" || observed[0].ManagedStatus != "offline" {
		t.Fatalf("Hub-observed Spoke was not linked to managed device: %#v", observed[0])
	}
	agentHandler := NewAgentWSHandler(&config.Config{AuthToken: "global"}, database, nodeMgr, service.NewLogHub())
	if !agentHandler.authenticateAgent("branch-1", "spoke", created.Token) ||
		agentHandler.authenticateAgent("branch-1", "spoke", "global") ||
		agentHandler.authenticateAgent("branch-1", "hub", "global") {
		t.Fatal("managed spoke authentication policy violated")
	}
	if !agentHandler.authenticateAgent("new-hub", "hub", "global") {
		t.Fatal("legacy Hub global token compatibility lost")
	}

	ctx, recorder = managedSpokeTestContext(http.MethodPost, "/api/managed-spokes/branch-1/token/rotate", `{"protocol_address":"10.20.0.3/32"}`)
	ctx.Params = gin.Params{{Key: "id", Value: "branch-1"}}
	handler.RotateToken(ctx)
	if recorder.Code != http.StatusOK || agentHandler.authenticateAgent("branch-1", "spoke", created.Token) {
		t.Fatalf("old token remained valid: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var reboundAddress string
	if err := database.QueryRow(`SELECT advertised_ip FROM nodes WHERE id='branch-1'`).Scan(&reboundAddress); err != nil || reboundAddress != "10.20.0.3" {
		t.Fatalf("existing managed Spoke was not rebound: address=%q err=%v", reboundAddress, err)
	}
	ctx, recorder = managedSpokeTestContext(http.MethodPost, "/api/managed-spokes/branch-1/token/rotate", "")
	ctx.Params = gin.Params{{Key: "id", Value: "branch-1"}}
	handler.RotateToken(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("plain token rotation failed after binding: status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	ctx, recorder = managedSpokeTestContext(http.MethodPost, "/api/managed-spokes/branch-1/ha/mode", `{"mode":"manual","member":"bad member"}`)
	ctx.Params = gin.Params{{Key: "id", Value: "branch-1"}}
	handler.SetHAMode(ctx)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid HA member accepted: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	ctx, recorder = managedSpokeTestContext(http.MethodPost, "/api/managed-spokes/branch-1/ha/mode", `{"mode":"auto","member":"hub-primary"}`)
	ctx.Params = gin.Params{{Key: "id", Value: "branch-1"}}
	handler.SetHAMode(ctx)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("auto mode accepted a member: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	ctx, recorder = managedSpokeTestContext(http.MethodPost, "/api/managed-spokes/branch-1/ha/mode", `{"mode":"manual","member":"hub-primary"}`)
	ctx.Params = gin.Params{{Key: "id", Value: "branch-1"}}
	handler.SetHAMode(ctx)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("offline managed Spoke mode change status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	ctx, recorder = managedSpokeTestContext(http.MethodDelete, "/api/managed-spokes/branch-1", "")
	ctx.Params = gin.Params{{Key: "id", Value: "branch-1"}}
	handler.Delete(ctx)
	if recorder.Code != http.StatusNoContent || agentHandler.authenticateAgent("branch-1", "spoke", created.Token) {
		t.Fatalf("delete did not revoke enrollment: status=%d", recorder.Code)
	}
}
