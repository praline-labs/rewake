package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/control"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// The decisions another session makes about a run read the live set its
// session record carries (docs/v2/design-api.md, "Two levels: offered and
// live"); what the harness offers never stands in for it. Every session here
// stands on a harness that offers every capability the decision asks about.

// withLive records web's run with the live set given.
func withLive(t *testing.T, dir string, web registry.Session, live []string) {
	t.Helper()
	web.Live = live
	if err := registry.Update(dir, web); err != nil {
		t.Fatal(err)
	}
}

// A message to a run whose Wake is not live is accepted and left for its
// inbox, exit 3, with no wait for a delivery nothing can make.
func TestASendToARunWithoutALiveWakeIsAcceptedUnwoken(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	withLive(t, dir, web, withoutLive(allLive(), adapter.WakeCapability))
	t.Setenv(state.SessionEnv, "api")
	code, out, errOut := run("send", "web", "run the suite", "--wait", "5")
	if code != ExitPending || !strings.Contains(out+errOut, unwoken) {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	if sent := finishedFor(t, dir, "web"); len(sent) != 1 {
		t.Fatalf("the accepted letter is not in web's inbox: %v", sent)
	}
}

// A task or a question to a run whose turn boundary is not live fails, and
// nothing is sent; a note still goes. The harness offers the boundary, so
// only the live set refuses.
func TestATaskToARunWithoutALiveBoundaryFails(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	web.Role = role.Main.ID
	withLive(t, dir, web, withoutLive(allLive(), adapter.TurnBoundaryCapability))
	t.Setenv(state.SessionEnv, "api")
	for kind, args := range map[string][]string{
		"task":     {"send", "web", "run the suite", "--wait", "0"},
		"question": {"send", "web", "which port?", "--question", "--wait", "0"},
	} {
		code, _, errOut := run(args...)
		if code != ExitFailed || !strings.Contains(errOut, "web's turn boundary is not live") ||
			!strings.Contains(errOut, "a "+kind+" to it would never be answered; nothing was sent") {
			t.Errorf("%s: exit %d, %q", kind, code, errOut)
		}
	}
	if sent := finishedFor(t, dir, "web"); len(sent) != 0 {
		t.Fatalf("a refused send left %v", sent)
	}
	if code, _, errOut := run("send", "web", "merged", "--notify", "--wait", "0"); code != ExitPending {
		t.Fatalf("a note: exit %d, %q", code, errOut)
	}
}

// The boundary's live mark is what lets a task through, whatever the
// harness's offer says: the offer is read for the words of a refusal only.
func TestTheLiveBoundaryDecidesNotTheOffer(t *testing.T) {
	realReporting(t)
	unreported(t)
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	withLive(t, dir, web, allLive())
	t.Setenv(state.SessionEnv, "api")
	if code, _, errOut := run("send", "web", "run the suite", "--wait", "0"); code == ExitFailed {
		t.Fatalf("a task to a run whose boundary is live was refused by the offer: %q", errOut)
	}
}

// A compaction or an interrupt of a run whose Control is not live fails,
// exit 1, and nothing is asked: the harness offers both verbs.
func TestSteeringARunWithoutALiveControlFails(t *testing.T) {
	for _, action := range []string{"compact", "interrupt"} {
		t.Run(action, func(t *testing.T) {
			dir, worker, controlDir := steerWorld(t, "")
			withLive(t, dir, worker, withoutLive(allLive(), adapter.ControlCapability))
			seen := answerOnce(t, controlDir, func(r control.Request) string { return `{"id":"` + r.ID + `","outcome":"started"}` })
			code, _, errOut := steerRun(action, "worker")
			if code != ExitFailed || !strings.Contains(errOut, "worker's control is not live") || !strings.Contains(errOut, "nothing was asked") {
				t.Fatalf("exit %d, %q", code, errOut)
			}
			if request, ok := <-seen; ok {
				t.Fatalf("a request reached the worker: %+v", request)
			}
		})
	}
}
