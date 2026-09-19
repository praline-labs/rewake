package wrap

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

type observedFixture struct{ *backendFixture }

func (*observedFixture) SessionState() sessionstate.Snapshot {
	count := uint64(7)
	return sessionstate.Snapshot{Compactions: &count, Coverage: "observed"}
}

type observedHarness struct {
	fakeHarness
	backend *observedFixture
}

func (h *observedHarness) Launch(r harness.LaunchRequest) (harness.LaunchPlan, error) {
	plan, err := h.fakeHarness.Launch(r)
	plan.Backend = h.backend
	return plan, err
}

func TestWrapperCollectsStateForEveryRoleDespiteDisplayPolicy(t *testing.T) {
	for _, selected := range []role.Role{role.Main, role.Write, role.General} {
		t.Run(selected.ID, func(t *testing.T) {
			dir := stateDir(t)
			backend := &observedFixture{backendFixture: &backendFixture{marker: filepath.Join(t.TempDir(), "start"), done: make(chan struct{})}}
			// The ordinary child runs independently while the snapshot worker persists.
			fake := &observedHarness{fakeHarness: fakeHarness{script: "sleep 0.3"}, backend: backend}
			code, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Role: selected})
			if err != nil || code != 0 {
				t.Fatalf("wrapper=%d %v", code, err)
			}
			files, err := filepath.Glob(filepath.Join(dir, "observations", "*.json"))
			if err != nil || len(files) != 1 {
				t.Fatalf("observations=%v %v", files, err)
			}
			raw, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatal(err)
			}
			var snapshot sessionstate.Snapshot
			if json.Unmarshal(raw, &snapshot) != nil || snapshot.Epoch == "" || snapshot.Compactions == nil || *snapshot.Compactions != 7 {
				t.Fatal("snapshot not scoped to wrapper epoch")
			}
		})
	}
}
