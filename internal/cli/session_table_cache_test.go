package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
)

func TestSessionTableShowsCacheOnlyWhenSomeSessionReportsIt(t *testing.T) {
	plain := session.Unknown("e")
	cached := session.Unknown("e")
	cached.Cache = session.NewCache(900, 50, 50)
	main := sessionView{Telemetry: &plain, Session: registry.Session{Name: "main-claude", Role: "main", Harness: "claude", CWD: "/p"}}
	worker := sessionView{Telemetry: &cached, Session: registry.Session{Name: "api-claude", Role: "write", Harness: "claude", CWD: "/p"}}

	without := strings.Join(sessionTable("r", []sessionView{main}, true), "\n")
	if strings.Contains(without, "Cache") {
		t.Fatalf("a room whose sessions report no cache grew a column:\n%s", without)
	}
	with := strings.Join(sessionTable("r", []sessionView{main, worker}, true), "\n")
	if !strings.Contains(with, "Cache") || !strings.Contains(with, "90% (stale)") {
		t.Fatalf("no Cache column:\n%s", with)
	}
}
