package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
)

func TestSessionTableShowsTypeOnlyWhenSomeSessionHasOne(t *testing.T) {
	main := sessionView{Session: registry.Session{Name: "main-claude", Role: "main", Harness: "claude", CWD: "/p"}}
	typed := sessionView{Type: "review", Session: registry.Session{Name: "api-claude", Role: "write", Harness: "claude", CWD: "/p"}}

	without := strings.Join(sessionTable("r", []sessionView{main}, false), "\n")
	if strings.Contains(without, "Type") {
		t.Fatalf("a room with no typed worker grew a column:\n%s", without)
	}
	with := strings.Split(strings.Join(sessionTable("r", []sessionView{main, typed}, false), "\n"), "\n")
	if !strings.Contains(with[len(with)-3], "Type") {
		t.Fatalf("no Type column:\n%s", strings.Join(with, "\n"))
	}
	if f := strings.Fields(with[len(with)-2]); f[2] != "-" {
		t.Fatalf("main's type is %q, want -", f[2])
	}
	if f := strings.Fields(with[len(with)-1]); f[2] != "review" {
		t.Fatalf("worker's type is %q, want review", f[2])
	}
}
