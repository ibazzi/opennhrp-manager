package service

import (
	"testing"
	"time"

	"opennhrp-manager/internal/protocol"
)

func TestLogHubPublishesToLiveSubscriber(t *testing.T) {
	hub := NewLogHub()
	entries, unsubscribe := hub.Subscribe()
	defer unsubscribe()

	want := protocol.LogEntry{NodeID: "hub-a", Message: "ready"}
	hub.Broadcast(want)
	select {
	case got := <-entries:
		if got.NodeID != want.NodeID || got.Message != want.Message {
			t.Fatalf("unexpected log entry: %+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("live subscriber did not receive log entry")
	}
}
