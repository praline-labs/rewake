package cli

import (
	"encoding/json"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func TestReviewEmptyShellListRetainsBaselineJSON(t *testing.T) {
	dir := liveSession(t, "temporary")
	if err := registry.Remove(dir, "temporary"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, "")
	t.Setenv(state.EpochEnv, "")
	code, output, stderr := run("list", "--json")
	if code != ExitOK {
		t.Fatal(code, stderr)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(output), &fields); err != nil {
		t.Fatal(err)
	}
	t.Logf("sessions=%s", fields["sessions"])
	if string(fields["sessions"]) != "null" {
		t.Fatal("empty shell list changed its baseline sessions:null JSON value")
	}
}
