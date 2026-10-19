package claude

import (
	"os"
	"testing"
)

// Every test runs in a private home: a worker's launch makes the worker
// profile's folder there, and the person's own must never be written.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "rewake-claude-home-")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("HOME", home)
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
