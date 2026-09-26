package ui

import (
	"os"
	"testing"

	"ircgo/internal/history"
)

// TestMain disables channel logging for ui tests so addLine behaves exactly
// as it did before logging existed. Tests that exercise logging itself
// (history_test.go) point history.LogDir at their own temp dir.
func TestMain(m *testing.M) {
	history.LogDir = ""
	os.Exit(m.Run())
}
