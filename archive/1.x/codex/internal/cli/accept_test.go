package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/control"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/sessionstate"
	"github.com/praline-labs/rewake/internal/state"
)

// acceptWorld is a Codex worker whose control directory is served, asked from
// the person's shell, outside any session.
func acceptWorld(t *testing.T, workerHarness string) (string, registry.Session, string) {
	t.Helper()
	dir, worker, controlDir := steerWorld(t, workerHarness)
	t.Setenv(state.SessionEnv, "")
	saved := steerLimits
	steerLimits = map[string]control.Limits{control.Accept: {Pickup: 300 * time.Millisecond, Outcome: 300 * time.Millisecond, Poll: 5 * time.Millisecond}}
	t.Cleanup(func() { steerLimits = saved })
	return dir, worker, controlDir
}

func TestAcceptAsksTheWorkersWrapper(t *testing.T) {
	_, _, controlDir := acceptWorld(t, "codex")
	seen := answerOnce(t, controlDir, func(r control.Request) string { return `{"id":"` + r.ID + `","outcome":"done"}` })
	code, out, errOut := run("accept", "worker", "Y")
	if code != ExitOK || !strings.Contains(out, "worker takes deliveries in Y now") {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	if request := <-seen; request.Action != control.Accept || request.Conversation != "Y" {
		t.Fatalf("the worker read %+v", request)
	}
}

func TestAcceptRefusedByTheWrapperFails(t *testing.T) {
	_, _, controlDir := acceptWorld(t, "codex")
	answerOnce(t, controlDir, func(r control.Request) string {
		return `{"id":"` + r.ID + `","outcome":"refused","reason":"` + control.NotSelected + `","detail":"the selected conversation is Z"}`
	})
	code, _, errOut := run("accept", "worker", "Y")
	if code != ExitFailed || !strings.Contains(errOut, "Y is not the conversation worker has selected") || !strings.Contains(errOut, "Z") {
		t.Fatalf("exit %d, %q", code, errOut)
	}
}

// The person accepts, not an agent: inside a session the call is wrong.
func TestAcceptIsRefusedInsideASession(t *testing.T) {
	acceptWorld(t, "codex")
	t.Setenv(state.SessionEnv, "lead")
	if code, _, errOut := run("accept", "worker", "Y"); code != ExitUsage || !strings.Contains(errOut, "outside any rewake session") {
		t.Fatalf("exit %d, %q", code, errOut)
	}
}

func TestAcceptRefusesAHarnessThatHoldsNothing(t *testing.T) {
	acceptWorld(t, "claude")
	if code, _, errOut := run("accept", "worker", "Y"); code != ExitUsage || !strings.Contains(errOut, "holds no deliveries") {
		t.Fatalf("exit %d, %q", code, errOut)
	}
}

func TestAcceptNeedsBothArguments(t *testing.T) {
	acceptWorld(t, "codex")
	if code, _, _ := run("accept", "worker"); code != ExitUsage {
		t.Fatalf("exit %d", code)
	}
}

// A worker in a conversation the launch did not ask for does not read its
// mail there until the person accepts it. The wrapper's own record decides,
// not the snapshot, which is published on a tick and may not show the hold
// yet (acceptance of September 29, 2026, finding 2).
func TestInboxWaitsForTheAcceptance(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	current, _ := registry.Lookup(dir, "api")
	rawUnread(t, dir, "api", map[string]any{"from": "web", "toEpoch": current.Epoch(), "text": "keep me"})
	if err := sessionstate.HoldMail(dir, "api", current.Epoch(), "the launch asked to resume X"); err != nil {
		t.Fatal(err)
	}
	refused := func(want string) {
		t.Helper()
		for _, args := range [][]string{{"inbox"}, {"inbox", "--peek"}, {"inbox", "--owed"}} {
			if code, out, errOut := run(args...); code != ExitFailed || !strings.Contains(errOut, want) || strings.Contains(out, "keep me") {
				t.Fatalf("%v: exit %d, %q, %q", args, code, out, errOut)
			}
		}
	}
	// No snapshot published yet: the record alone refuses.
	refused("waits for its conversation: the launch asked to resume X")
	// A published hold names the conversations and the acceptance.
	snapshot := sessionstate.Unknown(current.Epoch())
	snapshot.DeliveryHold = &sessionstate.DeliveryHold{Reason: sessionstate.HoldUnintended, Expected: "X", Selected: "Y", Detail: "the launch asked to resume X, and the terminal selected Y"}
	if err := sessionstate.Save(dir, "api", current.Epoch(), snapshot); err != nil {
		t.Fatal(err)
	}
	refused("the terminal selected Y")
	if err := sessionstate.AdmitMail(dir, "api", current.Epoch()); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := run("inbox"); code != ExitOK || !strings.Contains(out, "keep me") {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
}

// A record that cannot be read is no admission.
func TestAnUnreadableAdmissionRefusesTheMail(t *testing.T) {
	dir := liveSession(t, "api")
	t.Setenv(state.SessionEnv, "api")
	current, _ := registry.Lookup(dir, "api")
	rawUnread(t, dir, "api", map[string]any{"from": "web", "toEpoch": current.Epoch(), "text": "keep me"})
	if err := sessionstate.HoldMail(dir, "api", current.Epoch(), "held"); err != nil {
		t.Fatal(err)
	}
	records, _ := filepath.Glob(filepath.Join(dir, "admission", "*.json"))
	if len(records) != 1 || os.WriteFile(records[0], []byte("{"), 0o600) != nil {
		t.Fatalf("records %v", records)
	}
	if code, out, errOut := run("inbox"); code != ExitFailed || strings.Contains(out, "keep me") || !strings.Contains(errOut, "could not tell") {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
}

// main sees why a worker's deliveries wait, in both forms of rewake list.
func TestListShowsAHold(t *testing.T) {
	dir, worker, _ := steerWorld(t, "codex")
	now := time.Now()
	snapshot := sessionstate.Unknown(worker.Epoch())
	snapshot.PublishedAt = &now
	snapshot.DeliveryHold = &sessionstate.DeliveryHold{Reason: sessionstate.HoldUnintended, Expected: "X", Selected: "Y", Detail: "the launch asked to resume X, and the terminal selected Y"}
	if err := sessionstate.Save(dir, worker.Name, worker.Epoch(), snapshot); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := run("list"); code != ExitOK || !strings.Contains(out, "Deliveries to worker wait: the launch asked to resume X") {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	if code, out, errOut := run("list", "--json"); code != ExitOK || !strings.Contains(out, `"deliveryHold": {`) || !strings.Contains(out, `"reason": "unintended-conversation"`) || !strings.Contains(out, `"selectedConversation": "Y"`) {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
}
