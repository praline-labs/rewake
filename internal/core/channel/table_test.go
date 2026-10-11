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
	name string
	// noTool starts the run without the tool, for reason.
	noTool bool
	reason string
	steps  []step
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
func descendant(e *Event)         { e.Descendant = true }
func ok(e *Event)                 { e.OK = true }
func class(c string) func(*Event) { return func(e *Event) { e.Class = c } }

// told folds an event at another time than its step's: delivered late.
func told(at Stamp) func(*Event) { return func(e *Event) { e.At = at } }

func issued(at time.Duration) func(*Event) {
	return func(e *Event) { e.Issued = stampAt(at).Boot }
}

const s = time.Second

func scenarios() []scenario {
	return []scenario{
		{
			// Silence before the timer's end is a wait, not a failure.
			name:    "nothing observed before the timer's end",
			steps:   []step{ev(10*s, TimerPassed)},
			display: "mail: tool starting",
		},
		{
			name:    "a hello, no call yet",
			steps:   []step{ev(2*s, Hello, gen(1))},
			display: "mail: tool connected, unused",
		},
		{
			name:    "two servers, one closes",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Hello, gen(2)), ev(3*s, Closed, gen(2), alive)},
			display: "mail: tool connected, unused",
		},
		{
			name:    "an old server's EOF after a newer hello",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Hello, gen(2)), ev(3*s, Closed, gen(1), alive)},
			display: "mail: tool connected, unused",
		},
		{
			name:    "the last of two servers closes, the older one",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Hello, gen(2)), ev(3*s, Closed, gen(2), alive), ev(4*s, Closed, gen(1), alive)},
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:04; shell unconfirmed",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name: "a connection's hello delivered after another connection's close",
			steps: []step{
				ev(2*s, Hello, gen(2)), ev(3*s, Closed, gen(2), alive),
				ev(4*s, Hello, gen(1), told(stampAt(1*s))),
			},
			display: "mail: tool connected, unused",
			// The close was told as a failure before the hello came.
			worker: []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name: "the same failure several times keeps one interval",
			steps: []step{
				ev(1*s, Hello, gen(1)), ev(2*s, Closed, gen(1), alive),
				ev(30*s, Hello, gen(2)), ev(40*s, Closed, gen(2), alive),
			},
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:02; shell unconfirmed; reconnected 2026-10-04 12:00:30, unused",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name:    "a refused hello from the harness's tree before any hello",
			steps:   []step{ev(2*s, HelloRefused, descendant)},
			display: "mail: tool failing (server refused) since 2026-10-04 12:00:02; shell unconfirmed",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			// The refusal happened before the hello, though told after it:
			// the failure is shown from its time, the hello as a reconnection.
			name:    "a refused hello delivered after a later hello",
			steps:   []step{ev(3*s, Hello, gen(1)), ev(4*s, HelloRefused, descendant, told(stampAt(2*s)))},
			display: "mail: tool failing (server refused) since 2026-10-04 12:00:02; shell unconfirmed; reconnected 2026-10-04 12:00:03, unused",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name:    "a stray process's refused hello while a server works",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s)), ev(3*s, HelloRefused)},
			display: "mail: tool",
		},
		{
			name:    "a server gone an hour after its last call",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s)), ev(3600*s, Closed, gen(1), alive)},
			display: "mail: tool failing (server gone) since 2026-10-04 13:00:00; shell unconfirmed; tool last worked 2026-10-04 12:00:02",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			// The failure names what was not observed, at the timer's end.
			name:    "no hello within the timer",
			steps:   []step{ev(15*s, TimerPassed)},
			display: "mail: tool failing (no hello observed) since 2026-10-04 12:00:15; shell unconfirmed",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name:    "a hello at the timer's very end",
			steps:   []step{ev(15*s, Hello, gen(1)), ev(16*s, TimerPassed)},
			display: "mail: tool connected, unused",
		},
		{
			name:    "a late hello after the timer, then a ticket",
			steps:   []step{ev(15*s, TimerPassed), ev(20*s, Hello, gen(1)), ev(30*s, Validated, issued(29*s))},
			display: "mail: tool",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing, FirstMainWorks},
		},
		{
			name:    "a hello during an open interval is noted, not told",
			steps:   []step{ev(16*s, TimerPassed), ev(21*s, Hello, gen(1))},
			display: "mail: tool failing (no hello observed) since 2026-10-04 12:00:15; shell unconfirmed; reconnected 2026-10-04 12:00:21, unused",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name: "the tool fails, then the shell fails, then the shell works",
			steps: []step{
				ev(1*s, Hello, gen(1)), ev(2*s, Closed, gen(1), alive), ev(3*s, ShellObserved, class(ShellReadOnly)),
				ev(4*s, ShellObserved, ok),
			},
			display: "mail: through the shell since 2026-10-04 12:00:04; tool failing (server gone)",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing, FirstMainNone, FirstMainShell},
		},
		{
			name: "no tool at launch, then the shell fails", noTool: true, reason: launchReason,
			steps:   []step{ev(1*s, ShellObserved, class(ShellReadOnly))},
			display: "mail: no working channel: no tool (no tool offered), shell state read-only",
			main:    []string{FirstMainNoTool, FirstMainNone},
		},
		{
			name: "a late shell success from a closed interval",
			steps: []step{
				ev(1*s, Hello, gen(1)), ev(2*s, Closed, gen(1), alive), ev(10*s, Validated, issued(9*s)),
				ev(15*s, Hello, gen(2)), ev(20*s, Closed, gen(2), alive),
				ev(21*s, ShellObserved, ok, told(stampAt(5*s))),
			},
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:20; shell unconfirmed; tool last worked 2026-10-04 12:00:10",
			worker:  []string{FirstWorkerFailing, FirstWorkerFailing}, main: []string{FirstMainFailing, FirstMainWorks, FirstMainFailing},
		},
		{
			name: "the block, then a disconnect, a refused hello, a hello or a timer",
			steps: []step{
				ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s)), ev(3*s, Denied), ev(4*s, Closed, gen(1), alive),
				ev(5*s, HelloRefused, descendant), ev(6*s, Hello, gen(2)), ev(30*s, TimerPassed),
			},
			display: "mail: tool denied by policy", main: []string{FirstMainDenied},
		},
		{
			name:    "a policy denial (signal of L4)",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s)), ev(3*s, Denied)},
			display: "mail: tool denied by policy", main: []string{FirstMainDenied},
		},
		{
			// The hook's denial is that call's own answer: nothing reaches
			// the record.
			name:    "a denial of one call by the person's hook",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s))},
			display: "mail: tool",
		},
		{
			// A timeout is the call's outcome, its receipt's: nothing reaches
			// the record while its connection lives.
			name:    "a call times out with its child alive",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Validated, issued(2*s))},
			display: "mail: tool",
		},
		{
			name:    "the tool committed, the answer lost, the server gone",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(6*s, Validated, issued(6*s)), ev(7*s, Closed, gen(1), alive)},
			display: "mail: tool failing (server gone) since 2026-10-04 12:00:07; shell unconfirmed; tool last worked 2026-10-04 12:00:06",
			worker:  []string{FirstWorkerFailing}, main: []string{FirstMainFailing},
		},
		{
			name:    "a long call started, a denial recorded, the old call answers",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(5*s, Denied), ev(9*s, Validated, issued(2*s))},
			display: "mail: tool denied by policy", main: []string{FirstMainDenied},
		},
		{
			name:    "a denial cleared by a later ticket",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(5*s, Denied), ev(9*s, Validated, issued(8*s))},
			display: "mail: tool", main: []string{FirstMainDenied, FirstMainWorks},
		},
		{
			name:    "a ticket issued between two denials",
			steps:   []step{ev(1*s, Denied), ev(3*s, Denied), ev(4*s, Validated, issued(2*s))},
			display: "mail: tool denied by policy", main: []string{FirstMainDenied},
		},
		{
			name:    "the harness exits",
			steps:   []step{ev(1*s, Hello, gen(1)), ev(2*s, Exited), ev(3*s, Closed, gen(1), alive)},
			display: "mail: tool connected, unused",
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
			record := New(!sc.noTool, sc.reason, stampAt(0))
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
