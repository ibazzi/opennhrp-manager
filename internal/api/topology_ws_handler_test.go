package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"opennhrp-manager/internal/config"
	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
	"opennhrp-manager/internal/service"
)

func TestTopologyWSImmediatelySendsCompleteSnapshot(t *testing.T) {
	database, err := db.InitDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	logHub := service.NewLogHub()
	mgr := service.NewNodeManager(&config.Config{}, database, logHub)
	for _, id := range []string{"hub-primary", "hub-backup"} {
		if _, err := database.Exec(`INSERT INTO nodes (id, name, type) VALUES (?, ?, 'hub')`, id, id); err != nil {
			t.Fatal(err)
		}
		mgr.CacheClusterStatus(id, &executor.ClusterStatusInfo{Member: id, Term: 7})
		mgr.CacheSpokes(id, "", []executor.SpokeInfo{{ProtocolAddress: "10.20.0.2/32"}})
		mgr.CacheHAStatus(id, &executor.ReplicationStatusInfo{}, []executor.InviteRecord{{}}, &executor.KeyStatusInfo{})
		if _, err := database.Exec(`
			WITH RECURSIVE history(n) AS (VALUES(1) UNION ALL SELECT n+1 FROM history WHERE n<8000)
			INSERT INTO witness_probes (target_node_id, probe_type, target_ip)
			SELECT ?, CASE n%3 WHEN 0 THEN 'l3_nbma' WHEN 1 THEN 'l4_port' ELSE 'agent_telemetry' END, '' FROM history
		`, id); err != nil {
			t.Fatal(err)
		}
	}
	witness := service.NewWitnessService(&config.Config{}, database, mgr)
	handler := NewTopologyWSHandler(mgr, witness, logHub, NewSpokeHandler(mgr, database))
	router := gin.New()
	finished := make(chan struct{}, 2)
	router.GET("/topology/ws", func(c *gin.Context) {
		defer func() { finished <- struct{}{} }()
		handler.HandleWS(c)
	})
	server := httptest.NewServer(router)
	defer server.Close()

	// No polling or heartbeat runs: both the initial connection and Hub switch
	// must receive a complete first frame without waiting for an update event.
	for _, id := range []string{"hub-primary", "hub-backup"} {
		t.Run(id, func(t *testing.T) {
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/topology/ws?node_id="+id+"&include_ha=1", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			// Less than the one-second flush interval; the first frame is immediate.
			if err := conn.SetReadDeadline(time.Now().Add(750 * time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			var message liveMessage
			started := time.Now()
			if err := conn.ReadJSON(&message); err != nil {
				t.Fatalf("missing initial snapshot: %v", err)
			}
			t.Logf("complete first snapshot received in %s", time.Since(started))
			s := message.Topology
			if message.Type != "topology" || s == nil || s.NodeID != id || s.Cluster == nil || s.Cluster.Member != id || s.Cluster.Term != 7 {
				t.Fatalf("wrong initial snapshot: %+v", message)
			}
			if len(s.Nodes) != 3 || len(s.Spokes) != 1 || len(s.SpokesByNode) != 2 || len(s.SLAMatrix) != 2 || s.Replication == nil || len(s.Invites) != 1 || s.KeyStatus == nil || s.WitnessQuorum.Mode == "" || s.Timestamp.IsZero() {
				t.Fatalf("incomplete initial snapshot: %+v", s)
			}
			if err := conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("closed connection kept topology and HA subscriptions alive")
			}
		})
	}
}
