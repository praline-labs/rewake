package cli

import (
	"slices"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// TestEveryChangeFormHoldsLeaseThroughItsLastEffect holds the scope of a
// change (docs/v2/stage3-state.md#what-is-a-writer): every documented change
// form but a launch, which holds its own lease for the wrapper's life, run on
// a populated room by a session of it while the test itself holds no lease,
// is refused no write for want of one — not by its handler, not by the
// shell's observation, not by a cleanup that would swallow the refusal — and
// holds no lease once it returns.
func TestEveryChangeFormHoldsLeaseThroughItsLastEffect(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "api")
	if code, _, errOut := run("send", "web", "set the scene", "--notify", "--wait", "1"); code != ExitOK && code != ExitPending {
		t.Fatalf("the scene's letter: %d %s", code, errOut)
	}
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(epochEnv, web.Epoch())
	launch := testTable().catalog.IDs()[0]
	// The scene is set; from here no write of the room has a lease but the
	// one the form takes.
	buildtest.Release(dir)
	if state.Leased(dir) {
		t.Fatal("the test still holds the room's lease")
	}
	ran := 0
	for _, form := range documentedForms(launch) {
		if form.want != accessChange || slices.Contains(form.argv, launch) {
			continue
		}
		argv := form.argv
		if argv[0] == "send" || argv[0] == "edit" {
			argv = append(slices.Clone(argv), "--wait", "1")
		}
		before := state.NoLeaseRefusals()
		started := time.Now()
		code, _, errOut := run(argv...)
		if elapsed := time.Since(started); elapsed > 10*time.Second {
			t.Errorf("%q took %s", argv, elapsed)
		}
		if refused := state.NoLeaseRefusals() - before; refused != 0 {
			t.Errorf("%q was refused %d writes for want of a lease: %d %s", argv, refused, code, errOut)
		}
		if state.Leased(dir) {
			t.Fatalf("%q returned holding the room's lease", argv)
		}
		ran++
	}
	if ran < 10 {
		t.Fatalf("only %d change forms ran", ran)
	}
}
