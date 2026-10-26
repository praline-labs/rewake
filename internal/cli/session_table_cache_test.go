package cli

import (
	"slices"
	"strings"
	"testing"
	"time"

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

func TestCacheCellShowsWhatTheCacheHasLeft(t *testing.T) {
	now := time.Now()
	at := func(d time.Duration) *session.CacheLife {
		e := now.Add(d)
		return &session.CacheLife{TTL: "1h", ExpiresAt: &e}
	}
	for _, c := range []struct {
		life *session.CacheLife
		want string
	}{
		{nil, ""},
		{&session.CacheLife{TTL: "1h"}, ""},
		{at(42*time.Minute - 10*time.Second), " 42m"},
		{at(20 * time.Second), " <1m"},
		{at(-time.Second), " —"},
	} {
		if got := cacheLeft(c.life, now); got != c.want {
			t.Fatalf("%+v: %q, want %q", c.life, got, c.want)
		}
	}
}

// The Thinking column stands after Effort and before Context, and with the
// Cache column each cell stays under its own header.
func TestSessionTableShowsThinkingOnlyWhenSomeSessionTellsIt(t *testing.T) {
	plain := session.Unknown("e")
	main := sessionView{Telemetry: &plain, Session: registry.Session{Name: "main-claude", Role: "main", Harness: "claude", CWD: "/p"}}
	if table := strings.Join(sessionTable("r", []sessionView{main}, true), "\n"); strings.Contains(table, "Thinking") {
		t.Fatalf("a room where nobody tells thinking grew a column:\n%s", table)
	}
	on := true
	told := session.Unknown("e")
	told.Thinking, told.Fresh, told.SettingsFresh = &on, true, true
	told.Cache = session.NewCache(900, 50, 50)
	worker := sessionView{Telemetry: &told, Session: registry.Session{Name: "api-claude", Role: "write", Harness: "claude", CWD: "/p"}}
	lines := strings.Split(strings.Join(sessionTable("r", []sessionView{main, worker}, true), "\n"), "\n")
	header, row := strings.Fields(lines[3]), strings.Fields(lines[5])
	want := []string{"Session", "Role", "Status", "Model", "Effort", "Thinking", "Context", "Cache", "Compactions", "Age"}
	if strings.Join(header, " ") != strings.Join(want, " ") {
		t.Fatalf("the header: %q", header)
	}
	if !slices.Contains(row, "on") || slices.Index(row, "on")+1 >= len(row) || row[slices.Index(row, "on")+1] != "unknown" {
		t.Fatalf("the thinking cell is not before the context:\n%s", strings.Join(lines, "\n"))
	}
	if got := thinkingCell(&plain); got != "-" {
		t.Fatalf("an untold thinking: %q", got)
	}
}
