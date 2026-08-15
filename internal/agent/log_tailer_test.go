package agent

import "testing"

func TestStripSystemLogPrefix(t *testing.T) {
	tests := map[string]string{
		"Aug 20 22:22:21 VM-4-11-ubuntu opennhrp[2270046]: Reloading managed HA state":     "Reloading managed HA state",
		"Thu Aug 20 22:27:58 2026 daemon.info opennhrp[88238]: [10.164.0.2] Peer inserted": "[10.164.0.2] Peer inserted",
		"opennhrp-ha[42]: three or more Active Hubs":                                       "three or more Active Hubs",
		"opennhrp_ha[43]: quorum updated":                                                  "quorum updated",
		"plain program output":                                                             "plain program output",
	}
	for input, want := range tests {
		if got := stripSystemLogPrefix(input); got != want {
			t.Errorf("stripSystemLogPrefix(%q) = %q, want %q", input, got, want)
		}
	}
}
