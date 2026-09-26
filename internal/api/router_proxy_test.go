package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/service"
)

func TestAgentClientIPThroughTrustedProxies(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	cfg := &config.Config{TrustedProxies: "10.0.0.8/31"}
	nodeMgr := service.NewNodeManager(cfg, database, nil)
	router := SetupRouter(cfg, database, nodeMgr, service.NewWitnessService(cfg, database, nodeMgr), nil)
	router.GET("/test-client-ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	for _, tc := range []struct {
		remote, forwardedFor, realIP, want string
	}{
		{"127.0.0.1:1234", "10.0.0.3", "", "10.0.0.3"},
		{"10.0.0.9:1234", "10.0.0.3, 10.0.0.8", "", "10.0.0.3"},
		{"203.0.113.9:1234", "10.0.0.3", "", "203.0.113.9"},
		{"127.0.0.1:1234", "", "10.0.0.3", "10.0.0.3"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/test-client-ip", nil)
		req.RemoteAddr = tc.remote
		req.Header.Set("X-Forwarded-For", tc.forwardedFor)
		req.Header.Set("X-Real-IP", tc.realIP)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if got := response.Body.String(); got != tc.want {
			t.Errorf("remote %s: client IP %q, want %q", tc.remote, got, tc.want)
		}
	}
}
