package service

import (
	"math"
	"testing"

	"opennhrp-manager/internal/db"
)

func TestWSHistoryDoesNotMaskInboundFailures(t *testing.T) {
	probes := []db.WitnessProbeRecord{
		{ProbeType: "agent_telemetry", Success: true},
		{ProbeType: "l3_nbma", LossRate: 1, Detail: "ICMP blocked by firewall"},
		{ProbeType: "l4_port", LossRate: 1, Detail: "TCP blocked by firewall"},
	}
	got := SummarizeNodeSLA("hub", probes, db.NodeRecord{}, AgentTelemetry{}, true, true)
	if got.L3Healthy || got.L4Healthy || !got.AgentHealthy || got.LatencySource != "ws" {
		t.Fatalf("WS history masked direct probes: %+v", got)
	}
}

func TestPingLossParsing(t *testing.T) {
	for _, tc := range []struct {
		output  string
		loss    float64
		success bool
	}{
		{"2 packets transmitted, 2 received, 0% packet loss\nrtt min/avg/max/mdev = 55/56/57/1 ms", 0, true},
		{"2 packets transmitted, 1 packets received, 50% packet loss", .5, true},
		{"2 packets transmitted, 0 received, 100% packet loss", 1, false},
		{"3 packets transmitted, 2 received, 33.3333% packet loss", .333333, true},
		{"ping: unknown host", 1, false},
		{"101% packet loss", 1, false},
	} {
		rtt, loss, ok := parsePingOutput(tc.output)
		if math.Abs(loss-tc.loss) > 1e-8 || ok != tc.success {
			t.Fatalf("%q: loss=%v success=%v", tc.output, loss, ok)
		}
		if tc.loss == 0 && rtt != 56 {
			t.Fatalf("RTT parsing regressed: %v", rtt)
		}
	}
}

func TestSLALossSources(t *testing.T) {
	probes := []db.WitnessProbeRecord{
		{ProbeType: "l3_nbma", Success: true, LossRate: .5},
		{ProbeType: "l3_nbma", Success: false, LossRate: 1},
		{ProbeType: "l3_nbma", Success: true, LossRate: 0},
		{ProbeType: "l3_nbma", Success: true, LossRate: 0, Detail: "ICMP blocked by firewall"},
		{ProbeType: "l4_port", Success: true},
	}
	got := SummarizeNodeSLA("hub", probes, db.NodeRecord{}, AgentTelemetry{}, false, true)
	if got.LossRate != .5 || got.LossSamples != 3 || got.LatencySource != "icmp" {
		t.Fatalf("partial losses or old fallback mixed: %+v", got)
	}
	probes = append(probes, db.WitnessProbeRecord{ProbeType: "l3_nbma", Success: false, LossRate: 1, Detail: "ICMP blocked by firewall"})
	got = SummarizeNodeSLA("hub", probes, db.NodeRecord{}, AgentTelemetry{}, false, true)
	if got.LossRate != .625 || got.LossSamples != 4 {
		t.Fatalf("discarded actual ICMP failure: %+v", got)
	}
	probes[0].Detail = "ICMP blocked by firewall"
	tel := AgentTelemetry{WSRttMs: 56, WSLossRate: .25, WSLossSamples: 4}
	got = SummarizeNodeSLA("hub", probes, db.NodeRecord{}, tel, true, true)
	if got.LossRate != .25 || got.LossSamples != 4 || got.LatencySource != "ws" {
		t.Fatalf("WS measurements not used: %+v", got)
	}
	got = SummarizeNodeSLA("hub", probes, db.NodeRecord{}, tel, true, false)
	if got.LossSamples != 0 || got.AgentHealthy || got.OverallState != "critical" {
		t.Fatalf("offline node reported healthy: %+v", got)
	}
	got = SummarizeNodeSLA("hub", nil, db.NodeRecord{ProbeMode: "agent_only"}, AgentTelemetry{}, true, true)
	if got.LossSamples != 0 || got.LatencySource != "ws" {
		t.Fatalf("unsampled Agent reported ICMP loss: %+v", got)
	}
}
