package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"opennhrp-manager/internal/protocol"
)

func TestWriteFileAtomicKeepsModeAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opennhrp.conf")
	if err := os.WriteFile(path, []byte("old\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("new\n")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	backup, _ := os.ReadFile(path + ".bak")
	info, _ := os.Stat(path)
	if string(got) != "new\n" || string(backup) != "old\n" || info.Mode().Perm() != 0600 {
		t.Fatalf("unexpected atomic write: data=%q backup=%q mode=%o", got, backup, info.Mode().Perm())
	}
}

func TestSpokeRejectsHASocketCommand(t *testing.T) {
	accepted := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		accepted <- conn
	}))
	defer server.Close()

	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConn.Close()
	serverConn := <-accepted
	defer serverConn.Close()
	_ = serverConn.SetReadDeadline(time.Now().Add(time.Second))

	client := &AgentClient{nodeType: "spoke", wsConn: clientConn}
	client.handleCommand(protocol.Envelope{ID: "ha-test", Payload: protocol.CommandRequest{
		TargetSocket: "opennhrp-ha", Command: "ha cluster show\n",
	}})

	var envelope protocol.Envelope
	if err := serverConn.ReadJSON(&envelope); err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(envelope.Payload)
	var response protocol.CommandResponse
	_ = json.Unmarshal(payload, &response)
	if response.Success || response.Error != "HA socket is unavailable for spoke nodes" {
		t.Fatalf("unexpected response: %+v", response)
	}
}
