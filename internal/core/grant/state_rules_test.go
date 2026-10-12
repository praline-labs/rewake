package grant

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

func realDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// The rules of a session's grants split the base from the root
// (docs/v2/stage3-state.md#the-roots): the whole base is the tree no grant
// reaches — 1.x's rooms beside the root among it — and the live sessions
// whose working directories a grant must not take are the root's.
func TestTheStateRulesCoverTheBaseAndTheRootsSessions(t *testing.T) {
	before := TempRoots
	TempRoots = func() []string { return nil }
	t.Cleanup(func() { TempRoots = before })
	base := realDir(t)
	root := filepath.Join(base, "v2")
	legacy := filepath.Join(base, "rooms", "default")
	room := filepath.Join(root, "rooms", "default")
	for _, dir := range []string{filepath.Join(legacy, "sessions"), filepath.Join(room, "sessions")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	work := realDir(t)
	live := registry.Session{Name: "api", ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: work, StartedAt: time.Now()}
	if err := registry.Publish(room, live); err != nil {
		t.Fatal(err)
	}

	rules := CurrentEnv(base, root, nil, nil).Rules()
	for path, want := range map[string]string{
		legacy: "rewake's state directory",
		root:   "rewake's state directory",
		work:   "where a live rewake session works",
	} {
		var refusal *Refusal
		if err := rules.Check(path, path, false); !errors.As(err, &refusal) || !strings.Contains(refusal.Message, want) {
			t.Errorf("%s: %v, want %q", path, err, want)
		}
	}
}
