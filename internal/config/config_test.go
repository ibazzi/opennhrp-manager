package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWitnessEnabledAcceptsOpenWRTBool(t *testing.T) {
	t.Setenv("WITNESS_ENABLED", "1")
	if !LoadConfig().WitnessEnabled {
		t.Fatal("WITNESS_ENABLED=1 must enable the witness")
	}
	t.Setenv("WITNESS_ENABLED", "0")
	if LoadConfig().WitnessEnabled {
		t.Fatal("WITNESS_ENABLED=0 must disable the witness")
	}
}

func TestAuthTokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("file-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTH_TOKEN", "")
	t.Setenv("AUTH_TOKEN_FILE", path)
	if cfg := LoadConfig(); cfg.AuthToken != "file-secret" ||
		cfg.JWTSecret != "file-secret" {
		t.Fatalf("token file was not loaded: %+v", cfg)
	}
}
