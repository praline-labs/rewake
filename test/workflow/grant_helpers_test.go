package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// What the grant scenarios share: a directory a grant may name, and main's own
// account of a grant and of a report.

// grantableDir makes a directory a grant may name: outside the worker's
// workspace, not directly in HOME, and not in a temporary directory, which
// the hard tier refuses (docs/grants.md#the-hard-tier) and where every case's
// own directories lie. So it goes in the user's cache, beside the harness
// versions the suite keeps there, and is removed with the case.
func grantableDir(t *testing.T) (string, error) {
	t.Helper()
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	base := filepath.Join(cache, "rewake", "workflow-grants")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(base, "case-")
	if err != nil {
		return "", err
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	granted := filepath.Join(dir, "lib")
	if err := os.Mkdir(granted, 0o700); err != nil {
		return "", err
	}
	// Resolved, as rewake resolves it: the case compares the path it sent
	// with the roots the fixture received.
	return filepath.EvalSymlinks(granted)
}

// reportedOn says whether main no longer waits on a task: its report came.
// Asked of main's own listing, the same on every column.
func reportedOn(c *Case, asks *requests, id string) bool {
	code, machine, ok := asks.ask(c, "inbox", "--awaited", "--json")
	var awaited struct {
		Recipients []struct {
			Messages []struct {
				ID string `json:"id"`
			} `json:"messages"`
		} `json:"recipients"`
	}
	if !ok || code != 0 || json.Unmarshal([]byte(machine), &awaited) != nil {
		return false
	}
	for _, recipient := range awaited.Recipients {
		for _, message := range recipient.Messages {
			if message.ID == id {
				return false
			}
		}
	}
	return true
}
