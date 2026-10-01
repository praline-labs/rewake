package wrap

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/cutover"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// A successor record that is there and cannot be read refuses the launch:
// the reports held for the name wait on it, and a launch that went on would
// leave them unknown for good. Nothing of the launch is written.
func TestALaunchRefusesAnUnreadableSuccessor(t *testing.T) {
	dir := stateDir(t)
	path := filepath.Join(dir, "runs", "api-fake", "successor")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := claimName(Request{Dir: dir, Name: "api", Harness: &fakeHarness{}}, os.Getpid(), selfStart(t), thisBoot(t), dir, noWriters); err == nil {
		t.Fatal("a launch went on over an unreadable successor record")
	}
	if _, err := registry.Load(dir, "api-fake"); !errors.Is(err, registry.ErrNotFound) {
		t.Errorf("the refused launch published: %v", err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "{" {
		t.Errorf("the successor record was written over: %q", raw)
	}
}

// The launch reads before it proves the earlier writers stopped: the dead
// record of the name, one of the earlier build that names no pid namespace,
// is still there when the proof reads it, and the proof refuses on it as out
// of sight. A pruning read before it would erase that evidence.
func TestALaunchLeavesTheRecordItsProofReads(t *testing.T) {
	for _, explicit := range []string{"api", ""} {
		t.Run("name="+explicit, func(t *testing.T) {
			dir := stateDir(t)
			prior := registry.Session{Name: "api-fake", ServicePID: 4194000, ServiceStart: 7}
			if explicit == "" {
				prior.Name = "general-fake"
			}
			raw, err := json.Marshal(prior)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(state.SessionPath(dir, prior.Name), raw, 0o600); err != nil {
				t.Fatal(err)
			}
			check := func(dir, name string) error { return cutover.Check(nil, dir, name) }
			_, err = claimName(Request{Dir: dir, Name: explicit, Harness: &fakeHarness{}}, os.Getpid(), selfStart(t), thisBoot(t), dir, check)
			var refused *cutover.RefusalError
			if !errors.As(err, &refused) || refused.Name != prior.Name {
				t.Fatalf("launch over a record out of sight: %v", err)
			}
			if _, err := registry.Load(dir, prior.Name); err != nil {
				t.Fatalf("the launch removed the record its proof reads: %v", err)
			}
		})
	}
}
