package cli

import (
	"encoding/json"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
	"github.com/iiiokojiadbi/rewake/internal/state"
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
	savedCount := countLimit
	countLimit = 300 * time.Millisecond
	t.Cleanup(func() { steerLimits, countLimit = saved, savedCount })
	return dir, worker, controlDir
}

// counted publishes the worker's telemetry as its wrapper would once the
// compaction a request asked for is counted, as the latest of its compactions,
// with one before it that nobody asked for.
func counted(t *testing.T, dir string, worker registry.Session, request string, sequence uint64) {
	t.Helper()
	now := time.Now()
	snapshot := sessionstate.Unknown(worker.Epoch())
	snapshot.PublishedAt = &now
	snapshot.CompactionEvents = []sessionstate.CompactionEvent{
		{Sequence: sequence - 1, ObservedAt: now},
		{Sequence: sequence, ObservedAt: now, RequestedBy: "lead", Request: request},
	}
	if err := sessionstate.Save(dir, worker.Name, worker.Epoch(), snapshot); err != nil {
		t.Error(err)
	}
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

// Main gets no notice of a compaction it asked for, so the answer carries what
// the notice would: the count, besides the tokens.
func TestCompactDoneReportsTheTokensAndTheCount(t *testing.T) {
	dir, worker, controlDir := steerWorld(t, "claude")
	seen := answerOnce(t, controlDir, func(r control.Request) string {
		counted(t, dir, worker, r.ID, 3)
		return `{"id":"` + r.ID + `","outcome":"done","tokensBefore":120000,"tokensAfter":9000}`
	})
	code, out, errOut := run("compact", "worker", "keep the plan")
	if code != ExitOK || out != "Rewake: compacted worker: 120000 tokens before, 9000 after (compaction 3).\n" {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	if request := <-seen; request.Action != control.Compact || request.Focus != "keep the plan" || request.From != "lead" {
		t.Fatalf("the worker read %+v", request)
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
	_, _, controlDir := steerWorld(t, "claude")
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
	dir, worker, controlDir := steerWorld(t, "codex")
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
	answerOnce(t, controlDir, func(r control.Request) string {
		counted(t, dir, worker, r.ID, 1)
		return `{"id":"` + r.ID + `","outcome":"done"}`
	})
	if code, out, errOut := run("compact", "worker"); code != ExitOK || out != "Rewake: compacted worker (compaction 1).\n" {
		t.Fatalf("without a focus: exit %d, %q, %q", code, out, errOut)
	}
}

// A compaction the telemetry has not counted in time is still done, and the
// answer says where the count will show rather than taking another
// compaction's number.
func TestCompactDoneBeforeItIsCounted(t *testing.T) {
	dir, worker, controlDir := steerWorld(t, "claude")
	answerOnce(t, controlDir, func(r control.Request) string {
		counted(t, dir, worker, "0123456789abcdef0123456789abcdef", 2)
		return `{"id":"` + r.ID + `","outcome":"done","tokensBefore":120000,"tokensAfter":9000}`
	})
	code, out, errOut := run("compact", "worker", "--json")
	var model steerModel
	if code != ExitOK || json.Unmarshal([]byte(out), &model) != nil || model.Outcome != control.Done || model.Compaction != nil {
		t.Fatalf("exit %d, %q, %q", code, out, errOut)
	}
	if line := steerLine(model); !strings.Contains(line, "has not counted it yet, and rewake list shows the count") {
		t.Fatalf("the line: %q", line)
	}
}
