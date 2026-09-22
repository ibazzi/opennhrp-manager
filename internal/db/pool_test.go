package db

import (
	"path/filepath"
	"testing"
)

func TestInitDBUsesSQLiteConnectionPool(t *testing.T) {
	database, err := InitDB(filepath.Join(t.TempDir(), "manager.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	if got := database.Stats().MaxOpenConnections; got != sqlitePoolSize {
		t.Fatalf("MaxOpenConnections = %d, want %d", got, sqlitePoolSize)
	}

	var journalMode string
	if err := database.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want %q", journalMode, "wal")
	}
}
