package api

import (
	"strconv"
	"time"
)

const agentWSLatencyTimeout = 5 * time.Second

// All methods run on the WebSocket reader, including the Pong handler.
type agentWSLatency struct {
	sent, received time.Time
	sequence       uint64
	pending        string
	ms             float64
	outcomes       [20]bool
	count, next    int
}

func (l *agentWSLatency) ping(now time.Time) string {
	if !l.sent.IsZero() && (now.Sub(l.sent) < time.Second ||
		(l.pending != "" && now.Sub(l.sent) < agentWSLatencyTimeout)) {
		return ""
	}
	l.expire(now)
	l.sequence++
	l.pending = strconv.FormatUint(l.sequence, 10)
	l.sent = now
	return l.pending
}

func (l *agentWSLatency) pong(payload string, now time.Time) {
	if l.pending == "" || payload != l.pending {
		return
	}
	l.expire(now)
	if l.pending == "" {
		return
	}
	l.pending = ""
	elapsed := now.Sub(l.sent)
	if elapsed <= 0 || elapsed >= agentWSLatencyTimeout {
		return
	}
	l.record(false)
	l.ms = float64(elapsed) / float64(time.Millisecond)
	l.received = now
}

func (l *agentWSLatency) rtt(now time.Time) float64 {
	if l.received.IsZero() || now.Sub(l.received) >= agentWSLatencyTimeout {
		return 0
	}
	return l.ms
}

func (l *agentWSLatency) record(timeout bool) {
	l.outcomes[l.next] = timeout
	l.next = (l.next + 1) % len(l.outcomes)
	if l.count < len(l.outcomes) {
		l.count++
	}
}

func (l *agentWSLatency) expire(now time.Time) {
	if l.pending != "" && now.Sub(l.sent) >= agentWSLatencyTimeout {
		l.pending = ""
		l.record(true)
	}
}

// Only completed probes count; an outstanding Ping is not a loss.
func (l *agentWSLatency) loss(now time.Time) (float64, int) {
	l.expire(now)
	missed := 0
	for _, timeout := range l.outcomes[:l.count] {
		if timeout {
			missed++
		}
	}
	if l.count == 0 {
		return 0, 0
	}
	return float64(missed) / float64(l.count), l.count
}
