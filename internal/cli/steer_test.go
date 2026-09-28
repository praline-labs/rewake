package cli

import (
	"encoding/json"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/control"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// steerWorld is a main session "lead" and a running worker whose control
// directory exists, with limits short enough for a test.
func steerWorld(t *testing.T, workerHarness string) (string, registry.Session, string) {
	t.Helper()
	dir := liveSession(t, "lead")
	markMain(t, dir, "lead")
	t.Setenv(state.SessionEnv, "lead")
	worker := otherRun(t, dir, "worker")
	if workerHarness != "claude" {
		worker.Harness = workerHarness
		if err := registry.Update(dir, worker); err != nil {
			t.Fatal(err)
		}
	}
	controlDir := registry.ControlFor(dir, worker.Name, worker.Epoch())
	if err := control.Prepare(controlDir); err != nil {
		t.Fatal(err)
	}
	saved := steerLimits
	fast := control.Limits{Pickup: 300 * time.Millisecond, Outcome: 300 * time.Millisecond, Poll: 5 * time.Millisecond}
	steerLimits = map[string]control.Limits{control.Compact: fast, control.Interrupt: fast}
	t.Cleanup(func() { steerLimits = saved })
	return dir, worker, controlDir
}

// answerOnce plays the worker's plugin for one request.
func answerOnce(t *testing.T, controlDir string, answer func(control.Request) string) <-chan control.Request {
	t.Helper()
	seen := make(chan control.Request, 1)
	go func() {
		defer close(seen)
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
			raw, err := os.ReadFile(control.RequestPath(controlDir))
			if err != nil {
				continue
			}
			var request control.Request
			if json.Unmarshal(raw, &request) != nil {
				return
			}
			_ = os.WriteFile(control.TakenPath(controlDir, request.ID), nil, 0o600)
			_ = os.WriteFile(control.AnswerPath(controlDir, request.ID), []byte(answer(request)), 0o600)
			seen <- request
			return
		}
	}()
	return seen
}

// The command ends once the compaction has started, not at its end: the
// result comes to main as a letter, which main's own wrapper owes from the
// record the command leaves.
func TestCompactStartedEndsTheCommand(t *testing.T) {
	dir, worker, controlDir := steerWorld(t, "claude")
	seen := answerOnce(t, controlDir, func(r control.Request) string { return `{"id":"` + r.ID + `","outcome":"started"}` })
	code, out, errOut := run("compact", "worker", "keep the plan")
	if code != ExitOK || out != "Rewake: the compaction of worker started; its result comes to you as a letter, with the token counts and its number.\n" {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	request := <-seen
	if request.Action != control.Compact || request.Focus != "keep the plan" || request.From != "lead" {
		t.Fatalf("the worker read %+v", request)
	}
	lead, err := registry.Lookup(dir, "lead")
	if err != nil {
		t.Fatal(err)
	}
	pending := control.PendingOf(dir, "lead")
	if len(pending) != 1 || pending[0].ID != request.ID || pending[0].Worker.Name != worker.Name || pending[0].Worker.Epoch() != worker.Epoch() || pending[0].AskerEpoch != lead.Epoch() {
		t.Fatalf("the record for main's letter: %+v", pending)
	}
}

// A request taken and not answered within the limit may still be carried
// out: its record stays, made before the request was written, and the answer
// promises the letter. The same holds for a command killed after the pickup,
// which never gets to remove it.
func TestACompactionTakenAndUnansweredKeepsItsRecord(t *testing.T) {
	dir, worker, controlDir := steerWorld(t, "claude")
	taken := make(chan string, 1)
	go func() {
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
			raw, err := os.ReadFile(control.RequestPath(controlDir))
			var request control.Request
			if err != nil || json.Unmarshal(raw, &request) != nil {
				continue
			}
			if pending := control.PendingOf(dir, "lead"); len(pending) != 1 || pending[0].ID != request.ID {
				taken <- "no record when the request was written: " + request.ID
				return
			}
			if _, release, held := control.Hold(dir, "lead", request.ID); held {
				release()
				taken <- "main's wrapper could take the record while the command asked: " + request.ID
				return
			}
			_ = os.WriteFile(control.TakenPath(controlDir, request.ID), nil, 0o600)
			taken <- request.ID
			return
		}
		close(taken)
	}()
	code, _, errOut := run("compact", worker.Name)
	id := <-taken
	if code != ExitFailed || !strings.Contains(errOut, "it may still be carried out); its result comes to you as a letter, whether it compacts or not.") {
		t.Fatalf("exit %d, %q", code, errOut)
	}
	if pending := control.PendingOf(dir, "lead"); len(pending) != 1 || pending[0].ID != id {
		t.Fatalf("the record for main's letter: %+v, want %s", pending, id)
	}
	_, release, held := control.Hold(dir, "lead", id)
	if !held {
		t.Fatal("the command still holds its record after it ended")
	}
	release()
}

// A failure the served side gives as final — nothing was carried out — is an
// answer like a refusal: main has it, the record goes and no letter is
// promised. One that leaves the outcome open keeps the record and promises it.
func TestAFinalFailureClosesTheRecord(t *testing.T) {
	for _, tc := range []struct {
		answer, line string
		kept         bool
	}{
		{`"outcome":"failed","detail":"compaction is disabled"`, "Rewake: the compaction failed on worker (compaction is disabled).\n", false},
		{`"outcome":"failed","detail":"no answer to the compaction request; it may still start","open":true`, "Rewake: the compaction failed on worker (no answer to the compaction request; it may still start); its result comes to you as a letter, whether it compacts or not.\n", true},
	} {
		dir, _, controlDir := steerWorld(t, "codex")
		answerOnce(t, controlDir, func(r control.Request) string { return `{"id":"` + r.ID + `",` + tc.answer + `}` })
		code, _, errOut := run("compact", "worker")
		if code != ExitFailed || errOut != tc.line {
			t.Fatalf("%s: exit %d, %q", tc.answer, code, errOut)
		}
		if pending := control.PendingOf(dir, "lead"); len(pending) == 1 != tc.kept || len(pending) > 1 {
			t.Fatalf("%s: the record for main's letter: %+v", tc.answer, pending)
		}
	}
}

// A command that could not leave the record says no letter comes, rather
// than promising one.
func TestCompactWithNoRecordPromisesNoLetter(t *testing.T) {
	_, _, controlDir := steerWorld(t, "claude")
	saved := remember
	remember = func(string, string, control.Pending) (func(), error) { return nil, os.ErrPermission }
	t.Cleanup(func() { remember = saved })
	answerOnce(t, controlDir, func(r control.Request) string { return `{"id":"` + r.ID + `","outcome":"started"}` })
	code, out, errOut := run("compact", "worker")
	if code != ExitOK || out != "Rewake: the compaction of worker is started; rewake could not note it for its letter (permission denied), so none comes: rewake list shows whether it compacted.\n" {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
}

// A start the served side did not see in time is said as it is: requested,
// with the letter still to come.
func TestCompactRequestedSaysSo(t *testing.T) {
	_, _, controlDir := steerWorld(t, "codex")
	answerOnce(t, controlDir, func(r control.Request) string {
		return `{"id":"` + r.ID + `","outcome":"requested","detail":"the server has not answered the request within 3s"}`
	})
	code, out, errOut := run("compact", "worker", "--json")
	var model steerModel
	if code != ExitOK || json.Unmarshal([]byte(out), &model) != nil || model.Outcome != control.Requested || model.TokensBefore != nil {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	if line := steerLine(model); line != "Rewake: the compaction of worker is requested (the server has not answered the request within 3s); its result comes to you as a letter, whether it compacts or not." {
		t.Fatalf("the line: %q", line)
	}
}

func TestInterruptDoneAsJSON(t *testing.T) {
	_, _, controlDir := steerWorld(t, "claude")
	answerOnce(t, controlDir, func(r control.Request) string { return `{"id":"` + r.ID + `","outcome":"done"}` })
	code, out, errOut := run("interrupt", "worker", "--json")
	var model steerModel
	if code != ExitOK || json.Unmarshal([]byte(out), &model) != nil || model.Outcome != control.Done || model.Action != control.Interrupt || model.Session != "worker" {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
}

// The answer says what the interrupted session's model is shown, which
// differs by harness: a line in its next notice on Claude Code, nothing of
// rewake's on Codex, which records the interrupt itself.
func TestTheInterruptAnswerSaysWhatTheModelIsShown(t *testing.T) {
	for workerHarness, want := range map[string]string{
		"claude": "reads stopped, and its next notice says you interrupted it.\n",
		"codex":  "reads stopped, and Codex records the interrupt in its model's history.\n",
	} {
		t.Run(workerHarness, func(t *testing.T) {
			_, _, controlDir := steerWorld(t, workerHarness)
			answerOnce(t, controlDir, func(r control.Request) string { return `{"id":"` + r.ID + `","outcome":"done"}` })
			if code, out, errOut := run("interrupt", "worker"); code != ExitOK || !strings.HasSuffix(out, want) {
				t.Fatalf("exit %d, %q, %q", code, out, errOut)
			}
		})
	}
}

func TestARefusalExitsOneAndNamesTheNextAction(t *testing.T) {
	dir, _, controlDir := steerWorld(t, "claude")
	answerOnce(t, controlDir, func(r control.Request) string {
		return `{"id":"` + r.ID + `","outcome":"refused","reason":"in a turn","detail":"$.session.compact: a turn is running (t1)"}`
	})
	code, _, errOut := run("compact", "worker")
	if code != ExitFailed || !strings.Contains(errOut, "worker refused the compaction: in a turn ($.session.compact: a turn is running (t1)). A compaction never waits") {
		t.Fatalf("exit %d, %q", code, errOut)
	}
	answerOnce(t, controlDir, func(r control.Request) string {
		return `{"id":"` + r.ID + `","outcome":"refused","reason":"no turn running"}`
	})
	code, out, _ := run("interrupt", "worker", "--json")
	var model steerModel
	if code != ExitFailed || json.Unmarshal([]byte(out), &model) != nil || model.Reason != control.NoTurn {
		t.Fatalf("exit %d, %q", code, out)
	}
	if pending := control.PendingOf(dir, "lead"); len(pending) != 0 {
		t.Fatalf("a refusal left a record for a letter: %+v", pending)
	}
}

func TestAWorkerThatTakesNothingIsNotAnswering(t *testing.T) {
	_, _, controlDir := steerWorld(t, "claude")
	code, _, errOut := run("interrupt", "worker")
	if code != ExitFailed || !strings.Contains(errOut, "refused the interrupt: not answering") || !strings.Contains(errOut, "plugin is not loaded") {
		t.Fatalf("exit %d, %q", code, errOut)
	}
	if _, err := os.Stat(control.RequestPath(controlDir)); err == nil {
		t.Fatal("the request stayed behind")
	}
	if err := os.RemoveAll(controlDir); err != nil {
		t.Fatal(err)
	}
	// A session with no directory has the plugin, only an older one: the next
	// step is a restart, not a question about --bare.
	if code, _, errOut := run("compact", "worker"); code != ExitFailed || !strings.Contains(errOut, "refused the compaction: no control directory") ||
		!strings.Contains(errOut, "restart that session with the current rewake") || strings.Contains(errOut, "--bare") {
		t.Fatalf("without a control directory: exit %d, %q", code, errOut)
	}
}

// Esc or Ctrl+C on main, while this command runs from its Bash tool, sends
// the command SIGTERM (docs/research-claude-control.md): the wait ends, the
// request is withdrawn, and the answer says the call was cut short — not that
// the worker is silent. Without the handler the signal would end this test
// binary.
func TestASIGTERMDuringPickupWithdrawsTheRequest(t *testing.T) {
	_, _, controlDir := steerWorld(t, "claude")
	steerLimits = map[string]control.Limits{control.Compact: {Pickup: 5 * time.Second, Outcome: 5 * time.Second, Poll: 5 * time.Millisecond}}
	go func() {
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			if _, err := os.Stat(control.RequestPath(controlDir)); err == nil {
				_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
				return
			}
		}
	}()
	started := time.Now()
	code, _, errOut := run("compact", "worker")
	if code != ExitFailed || !strings.Contains(errOut, "refused the compaction: cut short") || !strings.Contains(errOut, "ask again") ||
		strings.Contains(errOut, "--bare") || time.Since(started) > 2*time.Second {
		t.Fatalf("exit %d after %s, %q", code, time.Since(started), errOut)
	}
	if _, err := os.Stat(control.RequestPath(controlDir)); err == nil {
		t.Fatal("the request stayed behind")
	}
}

// Every wrong call is refused with exit 2, and nothing is written for the
// worker to take.
func TestWrongCallsAreRefusedBeforeAnythingIsSent(t *testing.T) {
	dir, _, controlDir := steerWorld(t, "claude")
	cases := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"no session", map[string]string{state.SessionEnv: ""}, []string{"compact", "worker"}, "only a main session may compact another, and this is not one"},
		{"not main", map[string]string{state.SessionEnv: "worker"}, []string{"interrupt", "lead"}, "only a main session may interrupt another; worker is a"},
		{"no target", nil, []string{"interrupt"}, "rewake interrupt needs the session"},
		{"unknown", nil, []string{"compact", "nobody"}, `No session named "nobody" is running.`},
		{"itself", nil, []string{"interrupt", "lead"}, "a session cannot interrupt itself"},
		{"empty focus", nil, []string{"compact", "worker", " "}, "the focus is empty"},
		{"stray", nil, []string{"interrupt", "worker", "now"}, "at most 1 positional"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for name, value := range tc.env {
				t.Setenv(name, value)
			}
			if tc.name == "not main" {
				worker, _ := registry.Lookup(dir, "worker")
				t.Setenv(state.EpochEnv, worker.Epoch())
			}
			code, _, errOut := run(tc.args...)
			if code != ExitUsage || !strings.Contains(errOut, tc.want) {
				t.Fatalf("exit %d, %q; want 2 and %q", code, errOut, tc.want)
			}
			if !strings.Contains(errOut, "rewake "+tc.args[0]+" <name>") {
				t.Fatalf("the refusal does not show the syntax: %q", errOut)
			}
			if entries, _ := os.ReadDir(controlDir); len(entries) != 0 {
				t.Fatalf("a wrong call wrote %v", entries)
			}
		})
	}
}

// unsteerable is a harness that takes no control requests.
type unsteerable struct{ harness.Harness }

func TestAHarnessThatCannotTakeItIsAWrongCall(t *testing.T) {
	_, _, controlDir := steerWorld(t, "codex")
	saved := findHarness
	findHarness = func(id string) (harness.Harness, bool) {
		found, ok := saved(id)
		return unsteerable{found}, ok
	}
	code, _, errOut := run("compact", "worker")
	findHarness = saved
	if code != ExitUsage || !strings.Contains(errOut, "worker is a Codex session, which does not take rewake compact yet") {
		t.Fatalf("exit %d, %q", code, errOut)
	}
	code, _, errOut = run("compact", "worker", "keep the plan")
	if code != ExitUsage || !strings.Contains(errOut, "focus not supported by Codex") {
		t.Fatalf("exit %d, %q", code, errOut)
	}
	if entries, _ := os.ReadDir(controlDir); len(entries) != 0 {
		t.Fatalf("a wrong call wrote %v", entries)
	}
	answerOnce(t, controlDir, func(r control.Request) string { return `{"id":"` + r.ID + `","outcome":"started"}` })
	if code, out, errOut := run("compact", "worker"); code != ExitOK || !strings.HasPrefix(out, "Rewake: the compaction of worker started;") {
		t.Fatalf("without a focus: exit %d, %q, %q", code, out, errOut)
	}
}
