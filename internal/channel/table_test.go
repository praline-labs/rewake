package channel

import (
	"strings"
	"testing"
	"time"
)

// The failure points of docs/mail-bridge-channel.md, each a sequence whose
// display and notices — and their count — are asserted against the text.

type step struct {
	at    time.Duration
	event Event
}

type scenario struct {
	name     string
	harness  Harness
	injected bool
	reason   string
	steps    []step
	// display is the line after the last step; worker and main the first
	// lines told, in order, planning after every step.
	display string
	worker  []string
	main    []string
}

func ev(at time.Duration, kind Kind, opts ...func(*Event)) step {
	e := Event{Kind: kind}
	for _, opt := range opts {
		opt(&e)
	}
	return step{at: at, event: e}
}

func gen(n uint64) func(*Event)   { return func(e *Event) { e.Generation = n } }
func alive(e *Event)              { e.Alive = true }
func ok(e *Event)                 { e.OK = true }
func class(c string) func(*Event) { return func(e *Event) { e.Class = c } }
func issued(at time.Duration) func(*Event) {
	return func(e *Event) { e.Issued = stampAt(at).Boot }
}

const s = time.Second

func scenarios() []scenario {
	return []scenario{
		{
			name: "a hello, no call yet", harness: Codex, injected: true,
			steps:   []step{ev(1*s, ThreadAdmitted), ev(2*s, Hello, gen(1))},
			display: "mail: tool connected, unused",
		},
		{
			name: "working, then calls not observed", harness: Claude, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(5*s, Validated, issued(4*s)), ev(60*s, NotObserved)},
			display: "mail: tool failing (calls not observed) since 2026-10-04 12:01:00; shell unconfirmed; tool last worked 2026-10-04 12:00:05",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name: "the same failure several times keeps one interval", harness: Codex, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Closed, gen(1), alive), ev(30*s, NotObserved), ev(40*s, NotObserved)},
			display: "mail: tool failing (calls not observed) since 2026-10-04 12:00:02; shell unconfirmed",
			// The class changed, so each was told once more; the start stayed.
			worker: []string{FirstWorkerFailing, FirstWorkerFailing}, main: []string{FirstMainFailing, FirstMainFailing},
		},
		{
			name: "the same class again tells nothing more", harness: Codex, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(20*s, NotObserved), ev(40*s, NotObserved)},
			display: "mail: tool failing (calls not observed) since 2026-10-04 12:00:20; shell unconfirmed",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name: "two servers, one closes", harness: Codex, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Hello, gen(2)), ev(3*s, Closed, gen(2), alive)},
			display: "mail: tool connected, unused",
		},
		{
			name: "an old server's EOF after a newer hello", harness: Codex, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Hello, gen(2)), ev(3*s, Closed, gen(1), alive)},
			display: "mail: tool connected, unused",
		},
		{
			name: "the last of two servers closes, whichever is older", harness: Codex, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Hello, gen(2)), ev(3*s, Closed, gen(2), alive), ev(4*s, Closed, gen(1), alive)},
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:04; shell unconfirmed",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name: "a hello told after another connection's close", harness: Codex, injected: true,
			steps: []step{
				ev(2*s, Hello, gen(2)), ev(3*s, Closed, gen(2), alive),
				ev(4*s, Hello, gen(1), func(e *Event) { e.At = stampAt(1 * s) }),
			},
			display: "mail: tool connected, unused",
			// The close was told as a failure before the hello came.
			worker: []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name: "a startup failure told after a later hello", harness: Codex, injected: true,
			steps: []step{
				ev(1*s, ThreadAdmitted), ev(3*s, Hello, gen(1)),
				ev(4*s, StartupFailed, func(e *Event) { e.At = stampAt(2 * s) }),
			},
			display: "mail: tool failing (command cannot start) since 2026-10-04 12:00:02; shell unconfirmed; reconnected 2026-10-04 12:00:03, unused",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name: "a stray refused hello while a server works", harness: Codex, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s)), ev(3*s, HelloRefused)},
			display: "mail: tool",
		},
		{
			name: "Claude Code's server gone between turns", harness: Claude, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s)), ev(3600*s, Closed, gen(1), alive)},
			display: "mail: tool not connected",
		},
		{
			name: "no hello within the timer", harness: Claude, injected: true,
			steps:   []step{ev(0, HarnessStarted), ev(15*s, TimerPassed)},
			display: "mail: tool failing (no hello observed) since 2026-10-04 12:00:15; shell unconfirmed",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name: "a hello at the timer's end is in time", harness: Claude, injected: true,
			steps:   []step{ev(0, HarnessStarted), ev(15*s, Hello, gen(1)), ev(16*s, TimerPassed)},
			display: "mail: tool connected, unused",
		},
		{
			name: "a startup failure while a server lives is none", harness: Codex, injected: true,
			steps:   []step{ev(1*s, ThreadAdmitted), ev(2*s, Hello, gen(1)), ev(3*s, StartupFailed)},
			display: "mail: tool connected, unused",
		},
		{
			name: "a late hello after the timer, then a ticket", harness: Claude, injected: true,
			steps:   []step{ev(0, HarnessStarted), ev(15*s, TimerPassed), ev(20*s, Hello, gen(1)), ev(30*s, Validated, issued(29*s))},
			display: "mail: tool",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing, FirstMainWorks},
		},
		{
			name: "tool fails, the shell fails, then the shell works", harness: Codex, injected: true,
			steps: []step{
				ev(1*s, Hello, gen(1)), ev(2*s, Closed, gen(1), alive), ev(3*s, ShellObserved, class(ShellReadOnly)),
				ev(4*s, ShellObserved, ok),
			},
			display: "mail: through the shell since 2026-10-04 12:00:04; tool failing (server gone)",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing, FirstMainNone, FirstMainShell},
		},
		{
			name: "no tool at launch, then the shell fails", harness: Claude, reason: "--no-mail-tool",
			steps:   []step{ev(1*s, ShellObserved, class(ShellReadOnly))},
			display: "mail: no working channel: no tool (--no-mail-tool), shell state read-only",
			main:    []string{FirstMainNoTool, FirstMainNone},
		},
		{
			name: "a late shell success from a closed interval", harness: Codex, injected: true,
			steps: []step{
				ev(1*s, Hello, gen(1)), ev(2*s, Closed, gen(1), alive), ev(10*s, Validated, issued(9*s)),
				ev(20*s, NotObserved), ev(21*s, ShellObserved, ok, func(e *Event) { e.At = stampAt(5 * s) }),
			},
			display: "mail: tool failing (calls not observed) since 2026-10-04 12:00:20; shell unconfirmed; tool last worked 2026-10-04 12:00:10",
			worker:  []string{FirstWorkerFailing, FirstWorkerFailing}, main: []string{FirstMainFailing, FirstMainWorks, FirstMainFailing},
		},
		{
			name: "a denial, then a disconnect, a hello and a timer", harness: Codex, injected: true,
			steps: []step{
				ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s)), ev(3*s, Denied), ev(4*s, Closed, gen(1), alive),
				ev(5*s, ThreadAdmitted), ev(6*s, Hello, gen(2)), ev(30*s, TimerPassed),
			},
			display: "mail: tool denied by policy", main: []string{FirstMainDenied},
		},
		{
			name: "a long call started, a denial, the old call answers", harness: Claude, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(5*s, Denied), ev(9*s, Validated, issued(2*s))},
			display: "mail: tool denied by policy", main: []string{FirstMainDenied},
		},
		{
			name: "a denial cleared by a later ticket", harness: Claude, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(5*s, Denied), ev(9*s, Validated, issued(8*s))},
			display: "mail: tool", main: []string{FirstMainDenied, FirstMainWorks},
		},
		{
			name: "a ticket issued between two denials", harness: Codex, injected: true,
			steps:   []step{ev(1*s, Denied), ev(3*s, Denied), ev(4*s, Validated, issued(2*s))},
			display: "mail: tool denied by policy", main: []string{FirstMainDenied},
		},
		{
			name: "the harness exits", harness: Codex, injected: true,
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Exited), ev(3*s, Closed, gen(1), alive)},
			display: "mail: tool connected, unused",
		},
		{
			name: "a hello during an open interval is noted, not told", harness: Codex, injected: true,
			steps:   []step{ev(1*s, ThreadAdmitted), ev(16*s, TimerPassed), ev(20*s, ThreadAdmitted), ev(21*s, Hello, gen(1))},
			display: "mail: tool failing (no hello observed) since 2026-10-04 12:00:16; shell unconfirmed; reconnected 2026-10-04 12:00:21, unused",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
	}
}

func TestMain(m *testing.M) {
	// Lines show the reader's local time; the expectations are written in UTC.
	time.Local = time.UTC
	m.Run()
}

func TestEachFailurePointShowsAndTellsWhatTheTextSays(t *testing.T) {
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			record := New(sc.harness, sc.injected, sc.reason, stampAt(0))
			var notices Notices
			worker := Recipient{Role: ToWorker, Name: "w", Epoch: "1"}
			main := Recipient{Role: ToMain, Name: "m", Epoch: "1"}
			told := map[string][]string{}
			plan := func(now time.Duration) {
				for _, to := range []Recipient{worker, main} {
					// An hour apart, so no window holds back what the
					// scenario is about.
					if p, ok := notices.Plan(&record, to, stampAt(now)); ok {
						told[to.Role] = append(told[to.Role], strings.SplitN(p.Body, "\n", 2)[0])
						notices.Settle(p.Seq, Landed, stampAt(now).Boot)
					}
				}
			}
			plan(0)
			for i, st := range sc.steps {
				e := st.event
				if e.At.Boot == 0 {
					e.At = stampAt(st.at)
				}
				record.Fold(e)
				plan(time.Duration(i+1) * 2 * time.Hour)
			}
			if got := Label(&record); got != sc.display {
				t.Errorf("display\n got %q\nwant %q", got, sc.display)
			}
			if strings.Join(told[ToWorker], "|") != strings.Join(sc.worker, "|") {
				t.Errorf("worker told %q, want %q", told[ToWorker], sc.worker)
			}
			if strings.Join(told[ToMain], "|") != strings.Join(sc.main, "|") {
				t.Errorf("main told %q, want %q", told[ToMain], sc.main)
			}
		})
	}
}
