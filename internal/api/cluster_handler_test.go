package api

import (
	"errors"
	"strings"
	"testing"
)

func TestMemberCommandError(t *testing.T) {
	message := memberCommandError(errors.New("agent command error: the current Leader must transfer leadership first"), "hub-backup1")
	if !strings.Contains(message, "hub-backup1") || !strings.Contains(message, "仍是 Leader") {
		t.Fatalf("unexpected message: %s", message)
	}
}
