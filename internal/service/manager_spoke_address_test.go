package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"opennhrp-manager/internal/db"
	"opennhrp-manager/internal/executor"
)

type spokeInterfaceExecutor struct {
	executor.NodeExecutor
	addresses []string
	err       error
}

func (e spokeInterfaceExecutor) GetNodeID() string { return "ctyun" }
func (e spokeInterfaceExecutor) GetInterfaces(context.Context) ([]executor.InterfaceInfo, error) {
	result := []executor.InterfaceInfo{{Name: "eth0", ProtocolAddress: "192.168.1.2/24"}}
	for _, address := range e.addresses {
		result = append(result, executor.InterfaceInfo{ProtocolAddress: address, Flags: "shortcut configured"})
	}
	return result, e.err
}

func TestSyncSpokeProtocolAddress(t *testing.T) {
	database, err := db.InitDB(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	_, err = database.Exec(`INSERT INTO nodes (id, name, type, advertised_ip) VALUES
	 ('ctyun', '天翼云', 'spoke', ''), ('other', 'Other', 'spoke', '10.164.0.25')`)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewNodeManager(nil, database, nil)
	for _, tc := range []struct {
		name      string
		addresses []string
		err       error
		want      string
	}{
		{"learn CIDR", []string{"10.164.0.26/24"}, nil, "10.164.0.26"},
		{"track change", []string{"10.164.0.27"}, nil, "10.164.0.27"},
		{"unavailable", nil, errors.New("offline"), "10.164.0.27"},
		{"empty", nil, nil, "10.164.0.27"},
		{"invalid", []string{"bad", "0.0.0.0", "127.0.0.1"}, nil, "10.164.0.27"},
		{"ambiguous", []string{"10.164.0.28", "10.164.0.29"}, nil, "10.164.0.27"},
		{"conflict", []string{"10.164.0.25"}, nil, "10.164.0.27"},
		{"same address on multiple interfaces", []string{"10.164.0.28", "10.164.0.28/24"}, nil, "10.164.0.28"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager.syncSpokeProtocolAddress(context.Background(), spokeInterfaceExecutor{addresses: tc.addresses, err: tc.err})
			var address, name string
			if err := database.QueryRow(`SELECT advertised_ip, name FROM nodes WHERE id='ctyun'`).Scan(&address, &name); err != nil {
				t.Fatal(err)
			}
			if address != tc.want || name != "天翼云" {
				t.Fatalf("address=%q name=%q, want address=%q", address, name, tc.want)
			}
		})
	}
}
