package channel

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The failure points of docs/mail-bridge-channel-failures.md that concern
// Codex's conversations, and the points where nothing reaches the record.

func thread(name string) func(*Event) { return func(e *Event) { e.Thread = name } }

// told folds an event at another time than its step's: delivered late.
func told(at Stamp) func(*Event) { return func(e *Event) { e.At = at } }

// conversationA is a run whose first thread A was selected, its server
// bound to it and working: the start the selection rows go on from.
func conversationA() []step {
	return []step{
		ev(1*s, SelectionAdmitted), ev(2*s, Hello, gen(1)), ev(3*s, Selected, thread("A")),
		ev(4*s, Bound, gen(1), thread("A")), ev(5*s, Validated, issued(5*s)),
	}
}

func after(start []step, more ...step) []step { return append(start, more...) }

// bLive is A working, then B's server live and bound to B before B is
// admitted.
func bLive() []step {
	return after(conversationA(), ev(10*s, Hello, gen(2)), ev(11*s, Bound, gen(2), thread("B")), ev(12*s, SelectionAdmitted, thread("B")))
}

const lastWorked = "; tool last worked 2026-10-04 12:00:05"

func codexScenarios() []scenario {
	failingWorker, failingMain := []string{FirstWorkerFailing}, []string{FirstMainFailing}
	return []scenario{
		{
			name: "a policy denial (signal of L4)", harness: Claude, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s)), ev(3*s, Denied)},
			display: "mail: tool denied by policy", main: []string{FirstMainDenied},
		},
		{
			// The hook's denial is that call's own answer: nothing reaches
			// the record.
			name: "a denial of one call by the person's hook", harness: Claude, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s))},
			display: "mail: tool",
		},
		{
			// A timeout is the call's outcome, its receipt's: nothing reaches
			// the record while its connection lives.
			name: "a call times out with its child alive", harness: Codex, injected: true,
			steps:   after(conversationA()),
			display: "mail: tool",
		},
		{
			name: "the tool committed, the answer lost, the server gone", harness: Codex, injected: true,
			steps:   after(conversationA(), ev(6*s, Validated, issued(6*s)), ev(7*s, Closed, gen(1), alive)),
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:07; shell unconfirmed; tool last worked 2026-10-04 12:00:06",
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "a Codex sub-agent calls the tool", harness: Codex, injected: true,
			steps:   after(conversationA(), ev(6*s, Hello, gen(2)), ev(7*s, Bound, gen(2), thread("sub"))),
			display: "mail: tool",
		},
		{
			name: "Codex: the parent's server closes, a sub-agent's bound earlier still lives", harness: Codex, injected: true,
			steps:   after(conversationA(), ev(6*s, Hello, gen(2)), ev(7*s, Bound, gen(2), thread("sub")), ev(10*s, Closed, gen(1), alive)),
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:10; shell unconfirmed" + lastWorked,
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "Codex: the parent's server closes, then the other connection's first call names a sub-agent", harness: Codex, injected: true,
			steps:   after(conversationA(), ev(6*s, Hello, gen(2)), ev(10*s, Closed, gen(1), alive), ev(12*s, Bound, gen(2), thread("sub"))),
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:10; shell unconfirmed" + lastWorked,
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "Codex: the parent's server closes, the other connection never calls", harness: Codex, injected: true,
			steps:   after(conversationA(), ev(6*s, Hello, gen(2)), ev(10*s, Closed, gen(1), alive)),
			display: "mail: tool",
		},
		{
			name: "Codex: a binding folded after events that followed it", harness: Codex, injected: true,
			steps: after(conversationA(), ev(6*s, Hello, gen(2)), ev(10*s, Closed, gen(1), alive),
				ev(12*s, Bound, gen(2), thread("sub"), told(stampAt(8*s)))),
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:10; shell unconfirmed" + lastWorked,
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "Codex: a foreign connection's hello while the timer waits for the primary's", harness: Codex, injected: true,
			steps: []step{
				ev(1*s, SelectionAdmitted), ev(2*s, Selected, thread("A")), ev(5*s, Hello, gen(1)),
				ev(6*s, Bound, gen(1), thread("sub")), ev(20*s, TimerPassed),
			},
			display: "mail: tool failing (no hello observed) since 2026-10-04 12:00:16; shell unconfirmed",
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "Codex: A working, B admitted at T1, selected at S, no hello of B", harness: Codex, injected: true,
			// The timer from T1 = 10 s passes at 25 s, before S = 40 s: B
			// fails from its end, told at S.
			steps:   after(conversationA(), ev(10*s, SelectionAdmitted), ev(30*s, TimerPassed), ev(40*s, Selected, thread("B"))),
			display: "mail: tool failing (no hello observed) since 2026-10-04 12:00:25; shell unconfirmed" + lastWorked,
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "Codex: the same, a failed startup status naming B at T2 before S", harness: Codex, injected: true,
			steps: after(conversationA(), ev(10*s, SelectionAdmitted), ev(11*s, StartupFailed, thread("B")),
				ev(12*s, Selected, thread("B"))),
			display: "mail: tool failing (command cannot start) since 2026-10-04 12:00:11; shell unconfirmed" + lastWorked,
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "Codex: the same, a failed status naming another thread, or naming none, before S", harness: Codex, injected: true,
			steps: after(conversationA(), ev(10*s, SelectionAdmitted), ev(11*s, StartupFailed, thread("X")),
				ev(13*s, StartupFailed), ev(14*s, Selected, thread("B"))),
			display: "mail: tool failing (command cannot start) since 2026-10-04 12:00:13; shell unconfirmed" + lastWorked,
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "Codex: B's hello at T2 before S", harness: Codex, injected: true,
			steps: after(conversationA(), ev(10*s, SelectionAdmitted), ev(11*s, Hello, gen(2)),
				ev(12*s, Selected, thread("B")), ev(40*s, TimerPassed)),
			display: "mail: tool connected, unused",
		},
		{
			name: "Codex: A's own server closes between T1 and S", harness: Codex, injected: true,
			steps:   after(conversationA(), ev(10*s, SelectionAdmitted), ev(11*s, Closed, gen(1), alive), ev(12*s, Selected, thread("B"))),
			display: "mail: tool starting",
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "Codex: a call of A admitted before T1 is not observed after S", harness: Codex, injected: true,
			steps:   after(bLive(), ev(13*s, Selected, thread("B")), ev(20*s, NotObserved, gen(1), thread("A"))),
			display: "mail: tool connected, unused",
		},
		{
			name: "Codex: a call of A admitted before T1 is not observed between T1 and S", harness: Codex, injected: true,
			steps:   after(conversationA(), ev(10*s, SelectionAdmitted, thread("B")), ev(11*s, NotObserved, gen(1), thread("A")), ev(12*s, Selected, thread("B"))),
			display: "mail: tool starting",
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "Codex: a ticket of A validated after S", harness: Codex, injected: true,
			steps:   after(bLive(), ev(13*s, Selected, thread("B")), ev(20*s, Validated, gen(1), thread("A"), issued(9*s))),
			display: "mail: tool connected, unused",
		},
		{
			name: "Codex: the selection refused, unknown or read-only, or the primary left empty", harness: Codex, injected: true,
			steps:   after(conversationA(), ev(10*s, SelectionAdmitted), ev(12*s, SelectionFailed), ev(40*s, TimerPassed)),
			display: "mail: tool",
		},
		{
			// A's own close is told at once and the hello held, but once the
			// answer selects A again both are A's, and by event time the
			// hello lived at the close (mail-bridge-channel.md, a hello folded
			// late). The failure told meanwhile is not taken back; main
			// hears the tool works again, as A was working.
			name: "Codex: a resume of A, an unbound connection's hello held before A's own close, then the answer", harness: Codex, injected: true,
			steps: after(conversationA(), ev(10*s, SelectionAdmitted, thread("A")), ev(11*s, Hello, gen(2)),
				ev(12*s, Closed, gen(1), alive), ev(13*s, Selected, thread("A"))),
			display: "mail: tool",
			worker:  failingWorker, main: []string{FirstMainFailing, FirstMainWorks},
		},
		{
			// A's server gone, A resumed: the hello of a second server bound
			// to A ends the start the answer proved expected, at its own
			// time, so its close is the failure shown, not the timer.
			name: "Codex: A's server gone, a resume of A, a second server of A seen and closed, then the answer", harness: Codex, injected: true,
			steps: after(conversationA(), ev(6*s, Closed, gen(1), alive), ev(10*s, SelectionAdmitted, thread("A")),
				ev(11*s, Hello, gen(2)), ev(12*s, Bound, gen(2), thread("A")), ev(13*s, Closed, gen(2), alive),
				ev(14*s, Selected, thread("A")), ev(26*s, TimerPassed)),
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:06; shell unconfirmed" + lastWorked +
				"; reconnected 2026-10-04 12:00:11, unused",
			worker: failingWorker, main: failingMain,
		},
		{
			name: "Codex: a resume of the current conversation", harness: Codex, injected: true,
			steps: after(conversationA(), ev(10*s, SelectionAdmitted, thread("A")), ev(12*s, Selected, thread("A")),
				ev(40*s, TimerPassed)),
			display: "mail: tool",
		},
		{
			name: "Codex: A working, B's server live and bound to B before T1, B admitted at T1, selected at S, no events between", harness: Codex, injected: true,
			steps: after(conversationA(), ev(6*s, Hello, gen(2)), ev(7*s, Bound, gen(2), thread("B")),
				ev(10*s, SelectionAdmitted, thread("B")), ev(12*s, Selected, thread("B")), ev(40*s, TimerPassed)),
			display: "mail: tool connected, unused",
		},
		{
			name: "Codex: the same, B's last server closes at T2 between T1 and S", harness: Codex, injected: true,
			steps: after(conversationA(), ev(6*s, Hello, gen(2)), ev(7*s, Bound, gen(2), thread("B")),
				ev(10*s, SelectionAdmitted, thread("B")), ev(11*s, Closed, gen(2), alive), ev(12*s, Selected, thread("B"))),
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:11; shell unconfirmed" + lastWorked,
			worker:  failingWorker, main: failingMain,
		},
		{
			name: "Codex: the answer delivered after B's hello and status, all timed before it", harness: Codex, injected: true,
			steps: after(conversationA(), ev(10*s, SelectionAdmitted), ev(20*s, Selected, thread("B"), told(stampAt(14*s))),
				ev(21*s, Hello, gen(2), told(stampAt(12*s))), ev(22*s, StartupFailed, thread("B"), told(stampAt(11*s)))),
			display: "mail: tool failing (command cannot start) since 2026-10-04 12:00:11; shell unconfirmed" + lastWorked +
				"; reconnected 2026-10-04 12:00:12, unused",
			worker: failingWorker, main: failingMain,
		},
	}
}

// elsewhere are the failure points another test holds, by its name: the
// keeper's writes, its held closes and its end, which the record alone
// does not show.
var elsewhere = map[string]string{
	"the inbox that would carry a notice cannot be written":                        "TestNoticesWaitForTheOneInFlight",
	"a shell-advising notice to the worker fixed, its write failed, then a denial": "TestAdviceFixedBeforeTheBlockIsDropped",
	"main restarts during a suppressed failure":                                    "TestANewMainGetsTheCurrentCategory",
	"the harness exits, or the wrapper accepts SIGTERM or SIGHUP":                  "TestNothingIsToldAfterTheHarnessExits",
	"a server closes, and the harness exits within a heartbeat":                    "TestACloseYoungerThanAHeartbeatWaits",
	"a close, then later failures, folded after them":                              "TestACloseFoldedAfterLaterFailuresLandsWhereItHappened",
}

// Every row of the failure table is held by a test: a scenario of the
// same name, or the test named for it, and no scenario stands for a row
// the table no longer has.
func TestEveryFailurePointOfTheTableIsHeld(t *testing.T) {
	table, err := os.ReadFile(filepath.Join("..", "..", "docs", "mail-bridge-channel-failures.md"))
	if err != nil {
		t.Fatal(err)
	}
	named := map[string]bool{}
	for _, sc := range append(scenarios(), codexScenarios()...) {
		named[sc.name] = true
	}
	tests := testSources(t)
	rows := 0
	for _, line := range strings.Split(string(table), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) < 3 || !strings.HasPrefix(line, "| ") || strings.TrimSpace(cells[1]) == "Point" {
			continue
		}
		point := strings.TrimSpace(cells[1])
		rows++
		switch test, ok := elsewhere[point]; {
		case named[point]:
		case ok && !regexp.MustCompile(`func `+test+`\(`).MatchString(tests):
			t.Errorf("%q names %s, which no test of the module defines", point, test)
		case !ok:
			t.Errorf("no scenario holds the failure point %q", point)
		}
	}
	if rows < 40 {
		t.Fatalf("read %d rows of the failure table", rows)
	}
}

// testSources are the test files of the packages the table's rows are
// held in.
func testSources(t *testing.T) string {
	t.Helper()
	var all strings.Builder
	for _, dir := range []string{".", filepath.Join("..", "wrap")} {
		files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			all.Write(raw)
		}
	}
	return all.String()
}

// Main's notice of Claude Code's server gone names the one way back, the
// person's /mcp; Codex's, which nothing brings back, does not.
func TestMainIsToldThePersonCanReconnectClaudeCodesServer(t *testing.T) {
	for _, harness := range []Harness{Claude, Codex} {
		r := New(harness, true, "", stampAt(0))
		for _, e := range []Event{
			{Kind: Hello, Generation: 1, At: stampAt(s)},
			{Kind: Validated, At: stampAt(2 * s), Issued: stampAt(2 * s).Boot},
			{Kind: Closed, Generation: 1, Alive: true, At: stampAt(3 * s)},
		} {
			r.Fold(e)
		}
		var n Notices
		main, _ := n.Plan(&r, Recipient{Role: ToMain, Name: "m", Epoch: "1"}, stampAt(4*s))
		worker, _ := n.Plan(&r, Recipient{Role: ToWorker, Name: "w", Epoch: "1"}, stampAt(4*s))
		lines := strings.Split(main.Body, "\n")
		if reconnect := len(lines) > 1 && lines[1] == MainReconnect; reconnect != (harness == Claude) {
			t.Errorf("%s: main told %q", harness, main.Body)
		}
		if strings.Contains(worker.Body, MainReconnect) {
			t.Errorf("%s: the worker was told the person's way back: %q", harness, worker.Body)
		}
	}
}
