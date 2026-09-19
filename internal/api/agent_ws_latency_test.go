package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestAgentWSLatency(t *testing.T) {
	now := time.Now()
	var latency agentWSLatency
	if latency.rtt(now) != 0 {
		t.Fatal("unsampled RTT")
	}
	payload := latency.ping(now)
	latency.pong("unsolicited", now.Add(time.Millisecond))
	if latency.rtt(now) != 0 {
		t.Fatal("accepted unmatched pong")
	}
	if latency.ping(now.Add(2*time.Second)) != "" {
		t.Fatal("overwrote pending ping")
	}
	latency.pong(payload, now.Add(56*time.Millisecond))
	if latency.rtt(now.Add(time.Second)) != 56 {
		t.Fatal("incorrect round trip")
	}
	latency.pong(payload, now.Add(100*time.Millisecond))
	if latency.rtt(now.Add(time.Second)) != 56 {
		t.Fatal("accepted duplicate pong")
	}
	next := latency.ping(now.Add(time.Second))
	if next == "" || next == payload {
		t.Fatal("missing unique probe")
	}
	if latency.rtt(now.Add(6*time.Second)) != 0 {
		t.Fatal("stale RTT")
	}
	replacement := latency.ping(now.Add(6 * time.Second))
	if replacement == "" {
		t.Fatal("did not retry expired ping")
	}
	latency.pong(next, now.Add(6*time.Second))
	if latency.rtt(now.Add(6*time.Second)) != 0 {
		t.Fatal("accepted old pong")
	}
	latency.pong(replacement, now.Add(11*time.Second))
	if latency.rtt(now.Add(11*time.Second)) != 0 {
		t.Fatal("accepted expired pong")
	}
}

func TestAgentWSLatencyRoundTrip(t *testing.T) {
	result := make(chan float64, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			result <- -1
			return
		}
		defer conn.Close()
		var latency agentWSLatency
		conn.SetPongHandler(func(payload string) error {
			latency.pong(payload, time.Now())
			return nil
		})
		now := time.Now()
		if err := conn.WriteControl(websocket.PingMessage, []byte(latency.ping(now)), now.Add(time.Second)); err != nil {
			result <- -1
			return
		}
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			result <- -1
			return
		}
		result <- latency.rtt(time.Now())
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetPingHandler(func(payload string) error {
		time.Sleep(30 * time.Millisecond)
		if err := conn.WriteControl(websocket.PongMessage, []byte(payload), time.Now().Add(time.Second)); err != nil {
			return err
		}
		return conn.WriteMessage(websocket.TextMessage, []byte("heartbeat"))
	})
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	conn.ReadMessage()
	if ms := <-result; ms < 30 || ms >= 2000 {
		t.Fatalf("unexpected measured RTT: %v", ms)
	}
}

func TestAgentWSTimeoutRate(t *testing.T) {
	var l agentWSLatency
	now := time.Now()
	if rate, n := l.loss(now); rate != 0 || n != 0 {
		t.Fatal("unsampled loss")
	}
	payload := l.ping(now)
	if _, n := l.loss(now.Add(time.Second)); n != 0 {
		t.Fatal("counted pending probe")
	}
	l.pong(payload, now.Add(56*time.Millisecond))
	l.pong(payload, now.Add(57*time.Millisecond))
	if rate, n := l.loss(now.Add(time.Second)); rate != 0 || n != 1 {
		t.Fatal("duplicate success")
	}
	payload = l.ping(now.Add(time.Second))
	if rate, n := l.loss(now.Add(6 * time.Second)); rate != .5 || n != 2 {
		t.Fatalf("missing timeout: %v/%v", rate, n)
	}
	l.pong(payload, now.Add(7*time.Second))
	if rate, n := l.loss(now.Add(7 * time.Second)); rate != .5 || n != 2 {
		t.Fatal("late Pong counted twice")
	}
	for i := 0; i < 20; i++ {
		at := now.Add(time.Duration(8+i) * time.Second)
		p := l.ping(at)
		l.pong(p, at.Add(time.Millisecond))
	}
	if rate, n := l.loss(now.Add(30 * time.Second)); rate != 0 || n != 20 {
		t.Fatalf("window did not roll: %v/%v", rate, n)
	}
	// A Pong arriving at the deadline counts as one timeout, not a success.
	p := l.ping(now.Add(31 * time.Second))
	l.pong(p, now.Add(36*time.Second))
	if rate, n := l.loss(now.Add(36 * time.Second)); rate != .05 || n != 20 {
		t.Fatalf("deadline handling: %v/%v", rate, n)
	}
}
