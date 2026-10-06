package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

// rewake list only informs, so a session whose record cannot be read is
// shown as a row whose state is unknown — in the text and in the machine
// form, where a caller can test it — rather than left out as if it did not
// run. The exit is the list's usual one, and the list stays a view: the
// record is not pruned or changed.
func TestListShowsAnUnreadableSessionAsUnknown(t *testing.T) {
	dir := liveSession(t, "api")
	unparseable, closed := state.SessionPath(dir, "web"), state.SessionPath(dir, "ops")
	if err := os.WriteFile(unparseable, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(closed, []byte(`{"name":"ops"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(closed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(closed, 0o600) })
	code, out, errOut := run("list")
	if code != ExitOK || !strings.Contains(out, "api") || !strings.Contains(out, "web  state unknown") || !strings.Contains(out, "ops  state unknown") {
		t.Fatalf("list: %d %s %s", code, out, errOut)
	}
	code, out, _ = run("list", "--json")
	var model listModel
	if err := json.Unmarshal([]byte(out), &model); code != ExitOK || err != nil || len(model.Sessions) != 1 || len(model.Unknown) != 2 {
		t.Fatalf("list --json: %d %s %v", code, out, err)
	}
	for _, session := range model.Unknown {
		if session.State != "unknown" || session.Error == "" {
			t.Fatalf("an unreadable session in the machine form: %+v", session)
		}
	}
	if raw, err := os.ReadFile(unparseable); err != nil || !bytes.Equal(raw, []byte("{")) {
		t.Fatalf("the list changed a record it could not read: %q %v", raw, err)
	}
	if _, err := os.Lstat(closed); err != nil {
		t.Fatalf("the list pruned a record it could not read: %v", err)
	}
}
