package cli

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func answerWorld(t *testing.T) (string, string) {
	t.Helper()
	dir, _, controlDir := steerWorld(t, "")
	saved := answerLimits
	answerLimits = control.Limits{Pickup: 300 * time.Millisecond, Outcome: 300 * time.Millisecond, Poll: 5 * time.Millisecond}
	t.Cleanup(func() { answerLimits = saved })
	return dir, controlDir
}

// Main's answer reaches the worker's served side as the request it names and
// the words main typed, and the line says whether it was taken.
func TestAnAnswerCarriesTheRequestAndTheWords(t *testing.T) {
	_, controlDir := answerWorld(t)
	seen := answerOnce(t, controlDir, func(r control.Request) string { return `{"id":"` + r.ID + `","outcome":"done"}` })
	code, out, errOut := steerRun("answer", "worker", "3f9a", "refuse", "not on this branch")
	if code != ExitOK || out != "Rewake: worker took your answer to request 3f9a.\n" {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	request := <-seen
	if request.Action != control.Reply || request.Ask != "3f9a" || request.From != "lead" || strings.Join(request.Choices, "|") != "refuse|not on this branch" {
		t.Fatalf("the request: %+v", request)
	}
	answerOnce(t, controlDir, func(r control.Request) string {
		return `{"id":"` + r.ID + `","outcome":"refused","reason":"answered first at the screen"}`
	})
	code, out, errOut = steerRun("answer", "worker", "3f9a", "once", "--json")
	if code != ExitFailed || !strings.Contains(out, `"reason": "answered first at the screen"`) || !strings.Contains(out, `"request": "3f9a"`) {
		t.Fatalf("a lost answer: exit %d, %q, %q", code, out, errOut)
	}
}

// Only main answers, and a call short of a choice is wrong; nothing is
// written for the worker either way.
func TestAnAnswerFromAnythingButMainIsRefused(t *testing.T) {
	dir, controlDir := answerWorld(t)
	code, _, errOut := steerRun("answer", "worker", "3f9a")
	if code != ExitUsage || !strings.Contains(errOut, "needs the worker, the request and a choice") {
		t.Fatalf("no choice: exit %d, %q", code, errOut)
	}
	worker, _ := registry.Lookup(dir, "worker")
	t.Setenv(state.SessionEnv, "worker")
	t.Setenv(state.EpochEnv, worker.Epoch())
	code, _, errOut = steerRun("answer", "lead", "3f9a", "once")
	if code != ExitUsage || !strings.Contains(errOut, "only a main session answers") {
		t.Fatalf("not main: exit %d, %q", code, errOut)
	}
	if entries, _ := os.ReadDir(controlDir); len(entries) != 0 {
		t.Fatalf("a wrong call wrote %v", entries)
	}
}
