package mail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func writeRaw(t *testing.T, path, raw string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

// closeToReading closes a record to reading for the rest of the test.
func closeToReading(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
}

// announcedUnread leaves a letter where a serving process leaves one it has
// announced: readable, and no longer waiting.
func announcedUnread(t *testing.T, dir string, message Message) Message {
	t.Helper()
	if err := state.EnsureSubdir(state.InboxPath(dir, message.To)); err != nil {
		t.Fatal(err)
	}
	if err := state.EnsureSubdir(state.UnreadPath(dir, message.To)); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(message)
	if err := os.WriteFile(filepath.Join(state.UnreadPath(dir, message.To), message.ID+".json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return message
}

// unreadable leaves a file in place that cannot be read.
func unreadable(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// Written, then closed to reads: a file already there keeps its mode
	// through a write.
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
}
