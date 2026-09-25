package inbox

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// windowFixture serves a mailbox with a window and records every notice with
// the moment it went out.
type windowFixture struct {
	t      *testing.T
	dir    string
	server *Server
	cancel context.CancelFunc
	done   chan struct{}

	mu      sync.Mutex
	notices []Message
	at      []time.Time
	// answer is what the harness says to each notice; delivered when unset.
	answer func(call int) Result
}

func serveWindow(t *testing.T, window Window) *windowFixture {
	t.Helper()
	f := &windowFixture{t: t, dir: stateDir(t), done: make(chan struct{})}
	f.server = &Server{Dir: f.dir, Name: "api", Epoch: "5.5", Window: window, Deliver: f.deliver}
	ready := make(chan struct{})
	f.server.Ready = func() { close(ready) }
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	go func() { defer close(f.done); f.server.Serve(ctx) }()
	t.Cleanup(f.stop)
	<-ready
	return f
}

func (f *windowFixture) deliver(_ context.Context, m Message) Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notices = append(f.notices, m)
	f.at = append(f.at, time.Now())
	if f.answer != nil {
		return f.answer(len(f.notices))
	}
	return Result{State: Delivered, Via: "fixture"}
}

func (f *windowFixture) stop() {
	f.cancel()
	<-f.done
}

func (f *windowFixture) put(kind Kind, text string) Message {
	f.t.Helper()
	m := message(text)
	m.Kind, m.ToEpoch = kind, "5.5"
	if err := Put(f.dir, m); err != nil {
		f.t.Fatal(err)
	}
	return m
}

// seen waits for the nth notice and answers every notice so far with its time.
func (f *windowFixture) seen(n int, within time.Duration) ([]Message, []time.Time) {
	f.t.Helper()
	deadline := time.Now().Add(within)
	for {
		f.mu.Lock()
		notices, at := append([]Message(nil), f.notices...), append([]time.Time(nil), f.at...)
		f.mu.Unlock()
		if len(notices) >= n {
			return notices, at
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("%d notice(s) after %s, want %d: %+v", len(notices), within, n, notices)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func members(notice Message) []Message {
	if len(notice.Batch) == 0 {
		return []Message{notice}
	}
	return notice.Batch
}

func TestNotesSecondsApartAreAnnouncedOnce(t *testing.T) {
	f := serveWindow(t, Window{Quiet: 2 * time.Second, Cap: 3500 * time.Millisecond})
	for index, text := range []string{"first left", "second left", "first available"} {
		if index > 0 {
			// Each gap is far outside the 150 ms first collection, and the
			// last letter comes after a quiet counted from the first would
			// have closed: the quiet is counted from the latest. A stall only
			// widens a gap, so the margin that matters is the gap's under the
			// quiet; under -race on a busy machine a 700 ms sleep once took
			// 1.9 s.
			time.Sleep(1100 * time.Millisecond)
		}
		f.put(Note, text)
	}
	f.seen(1, 5*time.Second)
	// Long enough for a split notice to follow the first.
	time.Sleep(600 * time.Millisecond)
	notices, _ := f.seen(1, 0)
	if len(notices) != 1 || len(members(notices[0])) != 3 {
		t.Fatalf("got %d notice(s), want one carrying three notes: %+v", len(notices), notices)
	}
}

func TestReportsAndNotesWaitTogether(t *testing.T) {
	f := serveWindow(t, Window{Quiet: time.Second, Cap: 2 * time.Second})
	f.put(Finished, "done with the smoke")
	time.Sleep(400 * time.Millisecond)
	f.put(Note, "second available")
	time.Sleep(400 * time.Millisecond)
	f.put(Stopped, "stopped at the keyboard")
	notices, _ := f.seen(1, 4*time.Second)
	if len(members(notices[0])) != 3 {
		t.Fatalf("a report and notes seconds apart split: %+v", notices)
	}
}

// A steady stream never gives a quiet second; the cap still announces what has
// gathered, from the first letter, and the rest goes in a later notice.
func TestAStreamIsAnnouncedAtTheCap(t *testing.T) {
	f := serveWindow(t, Window{Quiet: time.Second, Cap: 2 * time.Second})
	started := time.Now()
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()
		f.put(Note, "stream")
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				f.put(Note, "stream")
			}
		}
	}()
	defer close(stop)
	notices, at := f.seen(1, 5*time.Second)
	if waited := at[0].Sub(started); waited < 1500*time.Millisecond || waited > 2700*time.Millisecond {
		t.Fatalf("the first notice of a stream went out after %s, want about the 2s cap", waited)
	}
	if len(members(notices[0])) < 2 {
		t.Fatalf("the capped notice carried %d letter(s), want what gathered", len(members(notices[0])))
	}
}

// The cap counts from when a letter was written, not from when the server
// first saw it: a sender waits from the write, and a pass that sees the letter
// late must not push the notice past that sender's wait.
func TestALetterSeenLateKeepsItsCap(t *testing.T) {
	f := serveWindow(t, Window{Quiet: 2 * time.Second, Cap: 3 * time.Second})
	late := message("available")
	late.Kind, late.ToEpoch = Note, "5.5"
	late.CreatedAt = time.Now().Add(-2500 * time.Millisecond)
	put := time.Now()
	if err := Put(f.dir, late); err != nil {
		t.Fatal(err)
	}
	_, at := f.seen(1, 4*time.Second)
	// Due half a second after the put by its write; by the sight alone, two.
	if waited := at[0].Sub(put); waited > 1200*time.Millisecond {
		t.Fatalf("a letter written 2.5s before it was seen waited %s more, as if its cap began at the sight", waited)
	}
}

// Work is announced at once, and whatever waits for company goes with it. A
// kind this build does not know is work too, as AsksForWork reads it.
func TestWorkIsAnnouncedAtOnceAndTakesWaitingMail(t *testing.T) {
	for _, kind := range []Kind{Task, Question, Kind("future")} {
		t.Run(string(kind), func(t *testing.T) {
			f := serveWindow(t, Window{Quiet: 3 * time.Second, Cap: 5 * time.Second})
			f.put(Note, "available")
			// Late enough that the note is already waiting on its window.
			time.Sleep(400 * time.Millisecond)
			sent := time.Now()
			f.put(kind, "run the smoke")
			notices, at := f.seen(1, 5*time.Second)
			if waited := at[0].Sub(sent); waited > time.Second {
				t.Fatalf("a %s waited %s behind a note", kind, waited)
			}
			if len(members(notices[0])) != 2 {
				t.Fatalf("the waiting note did not go with the %s: %+v", kind, notices[0])
			}
		})
	}
}

// A report a waiting send --question takes as its answer is handed to that
// send at once: holding it would hold the send.
func TestAReservedAnswerDoesNotWait(t *testing.T) {
	f := serveWindow(t, Window{Quiet: 3 * time.Second, Cap: 5 * time.Second})
	release := reserve(t, f.dir, "q1")
	defer release()
	sent := time.Now()
	answer := message("port 8080")
	answer.Kind, answer.ToEpoch, answer.InReplyTo = Finished, "5.5", []string{"q1"}
	if err := Put(f.dir, answer); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(state.UnreadPath(f.dir, "api"), answer.ID+".json")
	for {
		if _, err := os.Stat(linked); err == nil {
			break
		}
		if time.Since(sent) > time.Second {
			t.Fatal("a reserved answer waited for company")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if notices, _ := f.seen(0, 0); len(notices) != 0 {
		t.Fatalf("a reserved answer was announced: %+v", notices)
	}
}

// While a letter waits, its sender can read why; and a session that ends in
// the window loses nothing a later reader could have: the report stays
// readable, the note is refused the way any undelivered mail is.
func TestASessionEndingInTheWindowKeepsTheReport(t *testing.T) {
	f := serveWindow(t, Window{Quiet: 3 * time.Second, Cap: 5 * time.Second})
	note := f.put(Note, "available")
	report := f.put(Finished, "done")
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, ok := ReadStatus(f.dir, "api", note.ID)
		if ok && status.State == Pending && status.Detail == collectingDetail(f.server.Window) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a waiting letter's status does not say why: %+v", status)
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.stop()
	if notices, _ := f.seen(0, 0); len(notices) != 0 {
		t.Fatalf("announced during the window: %+v", notices)
	}
	if status, _ := ReadStatus(f.dir, "api", note.ID); status.State != Failed {
		t.Fatalf("note after the session ended: %+v", status)
	}
	status, _ := ReadStatus(f.dir, "api", report.ID)
	available, err := AvailableUnread(f.dir, "api", "5.5")
	if !status.ReportAvailable || err != nil || len(available) != 1 || available[0].ID != report.ID {
		t.Fatalf("report after the session ended: status %+v, readable %v %v", status, available, err)
	}
}

// A letter tried and put off keeps its first arrival: its retry is not a new
// letter, and does not wait for company all over again.
func TestARetryDoesNotRestartTheWindow(t *testing.T) {
	// A cap far off, so that only the quiet could hold the retry: the letter's
	// own cap, counted from its write, would otherwise have run out by then.
	f := serveWindow(t, Window{Quiet: 2500 * time.Millisecond, Cap: 20 * time.Second})
	f.answer = func(call int) Result {
		if call == 1 {
			return Result{State: Pending, Detail: "not yet"}
		}
		return Result{State: Delivered, Via: "fixture"}
	}
	f.put(Note, "available")
	_, at := f.seen(2, 10*time.Second)
	// The retry is due retryInterval after the first try and goes on the next
	// pass; waiting for company again would add the whole quiet window.
	if gap := at[1].Sub(at[0]); gap > retryInterval+1600*time.Millisecond {
		t.Fatalf("the retry came %s after the first try, as if the letter had just arrived", gap)
	}
}

func TestABuildSetsTheWindow(t *testing.T) {
	standard := Window{Quiet: 3 * time.Second, Cap: 4 * time.Second}
	if got := builtWindow(standard, "", ""); got != standard {
		t.Fatalf("a build that sets nothing serves %+v, want %+v", got, standard)
	}
	if got, want := builtWindow(standard, "1500ms", "2s"), (Window{Quiet: 1500 * time.Millisecond, Cap: 2 * time.Second}); got != want {
		t.Fatalf("a build's window is %+v, want %+v", got, want)
	}
	for _, bad := range []string{"soon", "0s", "-1s"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("a build with builtCap=%q served a window", bad)
				}
			}()
			builtWindow(standard, "", bad)
		}()
	}
}
