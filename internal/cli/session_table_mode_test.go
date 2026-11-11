package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
)

// The worker type is gone from the table; the observed runtime work mode is
// its own column, there only for a reader allowed the session state and only
// when some session reported one.
func TestSessionTableShowsModeNotType(t *testing.T) {
	mode := "review"
	main := sessionView{Session: registry.Session{Name: "main-claude", Role: "main", Harness: "claude", CWD: "/p"}}
	worker := sessionView{Mode: &mode, Session: registry.Session{Name: "api-claude", Role: "write", Harness: "claude", CWD: "/p"}}

	without := strings.Join(sessionTable("r", []sessionView{main}, false), "\n")
	if strings.Contains(without, "Type") || strings.Contains(without, "Mode") {
		t.Fatalf("a room with no observed mode grew a column:\n%s", without)
	}
	hidden := strings.Join(sessionTable("r", []sessionView{main, worker}, false), "\n")
	if strings.Contains(hidden, "Mode") {
		t.Fatalf("a reader not shown the session state sees the mode:\n%s", hidden)
	}
	shown := strings.Split(strings.Join(sessionTable("r", []sessionView{main, worker}, true), "\n"), "\n")
	if strings.Contains(shown[len(shown)-1], "Type") || !strings.Contains(shown[len(shown)-3], "Mode") {
		t.Fatalf("no Mode column:\n%s", strings.Join(shown, "\n"))
	}
	if f := strings.Fields(shown[len(shown)-2]); f[2] != "-" {
		t.Fatalf("main's mode is %q, want -", f[2])
	}
	if f := strings.Fields(shown[len(shown)-1]); f[2] != "review" {
		t.Fatalf("worker's mode is %q, want review", f[2])
	}
}
