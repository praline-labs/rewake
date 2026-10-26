package claude

import (
	"os"
	"testing"
)

// Every test runs in a private home: a worker's launch makes the worker
// profile's folder there, and the person's own must never be written or read.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "rewake-claude-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", home)
	// A status line's input without a project falls back to this: the
	// person's own project layers must not reach a test's line.
	_ = os.Setenv("CLAUDE_PROJECT_DIR", home)
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
