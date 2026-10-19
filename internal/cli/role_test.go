package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// markMain turns the test's session into the main one.
func markMain(t *testing.T, dir, name string) {
	t.Helper()
	session, err := registry.Load(dir, name)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	session.Role = role.Main.ID
	encoded, _ := json.MarshalIndent(session, "", "  ")
	if err := os.WriteFile(state.SessionPath(dir, name), encoded, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestListNamesTheMainSession(t *testing.T) {
	dir := liveSession(t, "lead")
	markMain(t, dir, "lead")
	_, out, _ := run("list")
	if !strings.Contains(out, "Role") || !strings.Contains(out, "main") {
		t.Errorf("list = %q, want the main session marked", out)
	}
}

// The main session reads reports and reports nothing: reading records no wait,
// and the end of its turn sends nothing, even to a session that wrote to it.
func TestTheMainSessionReportsNothing(t *testing.T) {
	dir := liveSession(t, "api")
	markMain(t, dir, "api")
	web := otherRun(t, dir, "web")
	readFrom(t, dir, web)
	if waiting := mail.Waiters(dir, "api", epochOf(t, dir, "api")); len(waiting) != 0 {
		t.Errorf("waiting = %v, want none for the main session", waiting)
	}

	// Even a wait recorded some other way — by an older rewake, say — is not
	// reported.
	waits := filepath.Join(state.AwaitingPath(dir, "api"), epochOf(t, dir, "api"))
	if err := os.MkdirAll(waits, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(waits, "web"), []byte(web.Epoch()+" 1 m1"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	mustEndTurn(t, smokeEnd)
	if found := finishedFor(t, dir, "web"); len(found) != 0 {
		t.Errorf("web holds %v, want no report from the main session", found)
	}
}
