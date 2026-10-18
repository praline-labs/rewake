package space

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// opening is a record about to open, as rewake space open writes it before
// it starts anything.
func opening(t *testing.T, name string) Record {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	session, err := NewSessionID()
	if err != nil {
		t.Fatal(err)
	}
	return Record{
		ID: id, Name: name,
		Main:     Run{Name: "main-one", Run: "4242.1700.0f0e0d0c-0b0a-4908-8706-050403020100"},
		Worker:   Worker{Harness: "worker-harness", Name: name, Role: "write", SessionID: session},
		OpenedAt: time.Now().UTC(),
	}
}

func created(t *testing.T, dir, name string) Record {
	t.Helper()
	record := opening(t, name)
	if err := Create(dir, record); err != nil {
		t.Fatal(err)
	}
	return record
}

// seated is a record moved to open, its checkout a real directory.
func seated(t *testing.T, dir, name string) (Record, string) {
	t.Helper()
	record := created(t, dir, name)
	if _, err := SetHost(dir, record.ID, "6161.1750.0f0e0d0c-0b0a-4908-8706-050403020100"); err != nil {
		t.Fatal(err)
	}
	if _, err := Opened(dir, record.ID, "5151.1800.0f0e0d0c-0b0a-4908-8706-050403020100"); err != nil {
		t.Fatal(err)
	}
	checkout := t.TempDir()
	opened, err := SetCheckout(dir, record.ID, Checkout{Path: checkout, Branch: name})
	if err != nil {
		t.Fatal(err)
	}
	return opened, checkout
}

func observed(worker WorkerEnd, seen Seen) End {
	return End{At: time.Now().UTC(), Why: "closed by main", Worker: worker, Checkout: seen}
}

func onDisk(t *testing.T, dir, id string) string {
	t.Helper()
	raw, err := os.ReadFile(state.SpacePath(dir, id))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// A record is written only through the state package under the room's
// lease: without it, and in a view, the record and every step refuse and
// leave the room as it was.
func TestTheRecordIsWrittenOnlyUnderTheRoomsLease(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	unleased := buildtest.Established(t, base, "unleased")
	if err := Create(unleased, opening(t, "api")); !errors.Is(err, state.ErrNoLease) {
		t.Fatalf("a record without the lease: %v", err)
	}
	if _, err := os.Lstat(state.SpacesPath(unleased)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused record left %v", err)
	}

	dir := buildtest.Room(t, base, "held")
	record := created(t, dir, "api")
	before := onDisk(t, dir, record.ID)
	leave := state.EnterView()
	if err := Create(dir, opening(t, "web")); !errors.Is(err, state.ErrViewWrite) {
		t.Errorf("a record in a view: %v", err)
	}
	if _, err := BeginClosing(dir, record.ID, "closed by main"); !errors.Is(err, state.ErrViewWrite) {
		t.Errorf("a step in a view: %v", err)
	}
	if err := leave(); !errors.Is(err, state.ErrViewWrite) {
		t.Errorf("leaving the view: %v", err)
	}
	buildtest.Release(dir)
	if _, err := BeginClosing(dir, record.ID, "closed by main"); !errors.Is(err, state.ErrNoLease) {
		t.Errorf("a step once the lease is gone: %v", err)
	}
	if after := onDisk(t, dir, record.ID); after != before {
		t.Fatalf("a refused step changed the record:\n%s\n%s", before, after)
	}
}

// The states move in one order: opening, open, closing, then kept or
// closed; the space host is recorded before the worker's run, the checkout
// once, a step out of order is refused naming the state, and kept becomes
// closed only once its checkout is seen gone.
func TestTheStatesMoveInTheirOrder(t *testing.T) {
	dir := buildtest.Dir(t)
	record := created(t, dir, "api")
	var step *StepError
	if _, err := Opened(dir, record.ID, "5151.1800"); !errors.As(err, &step) || step.State != Opening {
		t.Fatalf("open before the space host: %v", err)
	}
	if _, err := SetHost(dir, record.ID, "6161.1750"); err != nil {
		t.Fatal(err)
	}
	if _, err := SetHost(dir, record.ID, "6262.1760"); !errors.As(err, &step) {
		t.Fatalf("a second space host: %v", err)
	}
	if got, err := Opened(dir, record.ID, "5151.1800"); err != nil || got.State != Open || got.Worker.Run != "5151.1800" {
		t.Fatalf("open: %+v, %v", got, err)
	}
	if _, err := SetCheckout(dir, record.ID, Checkout{}); !errors.As(err, &step) {
		t.Fatalf("an empty checkout: %v", err)
	}
	checkout := t.TempDir()
	if _, err := SetCheckout(dir, record.ID, Checkout{Path: checkout, Branch: "api"}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetCheckout(dir, record.ID, Checkout{Path: t.TempDir()}); !errors.As(err, &step) {
		t.Fatalf("a second checkout: %v", err)
	}
	if got, err := BeginClosing(dir, record.ID, "closed by main"); err != nil || got.State != Closing {
		t.Fatalf("closing: %+v, %v", got, err)
	}
	if got, err := BeginClosing(dir, record.ID, "main's harness exited"); err != nil || got.Closing != "closed by main" {
		t.Fatalf("closing seen twice: %+v, %v", got, err)
	}
	if _, err := Opened(dir, record.ID, "5151.1800"); !errors.As(err, &step) || step.State != Closing {
		t.Fatalf("open again while closing: %v", err)
	}
	got, err := Close(dir, record.ID, observed(WorkerEnded, Seen{Presence: There, Changes: true}))
	if err != nil || got.State != Kept {
		t.Fatalf("closed with its checkout left: %+v, %v", got, err)
	}
	if _, err := Close(dir, record.ID, observed(WorkerEnded, Seen{Presence: Gone})); !errors.As(err, &step) || step.State != Kept {
		t.Fatalf("closed twice: %v", err)
	}
	if _, err := Cleared(dir, record.ID); err == nil || !strings.Contains(err.Error(), "is there, not gone") {
		t.Fatalf("cleared while the checkout is there: %v", err)
	}
	if err := os.Remove(checkout); err != nil {
		t.Fatal(err)
	}
	if got, err := Cleared(dir, record.ID); err != nil || got.State != Closed || got.End.Cleared == nil || got.End.Checkout.Presence != There {
		t.Fatalf("cleared once gone: %+v, %v", got, err)
	}
	if _, err := Cleared(dir, record.ID); !errors.As(err, &step) || step.State != Closed {
		t.Fatalf("cleared twice: %v", err)
	}

	// An open that fails before the worker starts closes from opening, its
	// worker never started and no checkout left; a worker that ran without a
	// checkout of its own closes with none.
	failed := created(t, dir, "web")
	if got, err := Close(dir, failed.ID, observed(WorkerNeverStarted, Seen{Presence: Gone})); err != nil || got.State != Closed {
		t.Fatalf("a failed open: %+v, %v", got, err)
	}
	plain := created(t, dir, "web")
	if _, err := SetHost(dir, plain.ID, "6161.1750"); err != nil {
		t.Fatal(err)
	}
	if _, err := Opened(dir, plain.ID, "5252.1810"); err != nil {
		t.Fatal(err)
	}
	if got, err := Close(dir, plain.ID, observed(WorkerEnded, Seen{Presence: Gone})); err != nil || got.State != Closed {
		t.Fatalf("a worker without a checkout: %+v, %v", got, err)
	}
}

// A record is closed only from what was observed (SP2): an end that says
// nothing, a worker seen to end that never had a run, one with a run said
// never to have started, a checkout left that the record never named — each
// is refused and the record stays as it was.
func TestARecordClosesOnlyFromAnObservation(t *testing.T) {
	dir := buildtest.Dir(t)
	unseated := created(t, dir, "api")
	open, _ := seated(t, dir, "web")
	for name, c := range map[string]struct {
		id  string
		end End
	}{
		"nothing observed":            {open.ID, End{}},
		"no time":                     {open.ID, End{Why: "closed by main", Worker: WorkerEnded}},
		"no worker's end":             {open.ID, End{At: time.Now(), Why: "closed by main"}},
		"a worker's end not observed": {open.ID, observed("stopped", Seen{Presence: Gone})},
		"an end without a run":        {unseated.ID, observed(WorkerEnded, Seen{Presence: Gone})},
		"a run that never started":    {open.ID, observed(WorkerNeverStarted, Seen{Presence: Gone})},
		"a checkout never named":      {unseated.ID, observed(WorkerNeverStarted, Seen{Presence: There})},
		"findings in no checkout":     {open.ID, observed(WorkerEnded, Seen{Presence: Gone, Commits: true})},
		"cleared at the close":        {open.ID, End{At: time.Now(), Why: "x", Worker: WorkerEnded, Checkout: Seen{Presence: There}, Cleared: new(time.Time)}},
	} {
		t.Run(name, func(t *testing.T) {
			before := onDisk(t, dir, c.id)
			if _, err := Close(dir, c.id, c.end); err == nil {
				t.Fatal("closed")
			}
			if after := onDisk(t, dir, c.id); after != before {
				t.Fatalf("a refused close changed the record:\n%s\n%s", before, after)
			}
		})
	}
}

// A crash between two steps leaves the earlier step whole, which a close
// from what was observed still ends; a crash at the first write leaves no
// record and nothing started.
func TestARecordLeftByACrashBetweenStepsCanBeClosed(t *testing.T) {
	dir := buildtest.Dir(t)
	crashAt := func(op string, at string) func() {
		state.Fault = func(o, path string) error {
			if o == op && filepath.Base(path) == at {
				return errors.New("crashed")
			}
			return nil
		}
		return func() { state.Fault = nil }
	}
	first := opening(t, "api")
	restore := crashAt(state.OpPublish, first.ID+".json")
	if err := Create(dir, first); err == nil {
		t.Fatal("the crashed record was written")
	}
	restore()
	if records, problems, err := List(dir); len(records) != 0 || len(problems) != 0 || err != nil {
		t.Fatalf("a crashed first write left %v %v %v", records, problems, err)
	}

	record := created(t, dir, "api")
	if _, err := SetHost(dir, record.ID, "6161.1750"); err != nil {
		t.Fatal(err)
	}
	restore = crashAt(state.OpWrite, record.ID+".json")
	if _, err := Opened(dir, record.ID, "5151.1800"); err == nil {
		t.Fatal("the crashed step was written")
	}
	restore()
	left, err := Load(dir, record.ID)
	if err != nil || left.State != Opening || left.Host != "6161.1750" {
		t.Fatalf("the crash left %+v, %v", left, err)
	}
	end := observed(WorkerNeverStarted, Seen{Presence: Gone})
	end.Why = "the space host could not record the worker's run"
	if got, err := Close(dir, record.ID, end); err != nil || got.State != Closed {
		t.Fatalf("the close after the crash: %+v, %v", got, err)
	}
}

// A name is free again once its space is over, and only then; a record that
// does not read may hold it, so it keeps every open from guessing.
func TestANameIsFreeOnlyOnceItsSpaceIsOver(t *testing.T) {
	dir := buildtest.Dir(t)
	first := created(t, dir, "api")
	var taken *NameTakenError
	if err := Create(dir, opening(t, "api")); !errors.As(err, &taken) || taken.ID != first.ID {
		t.Fatalf("a second open of the name: %v", err)
	}
	if _, err := Close(dir, first.ID, observed(WorkerNeverStarted, Seen{Presence: Gone})); err != nil {
		t.Fatal(err)
	}
	created(t, dir, "api")
	broken := state.SpacePath(dir, strings.Repeat("ab", 16))
	if err := os.WriteFile(broken, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Create(dir, opening(t, "web")); err == nil || !strings.Contains(err.Error(), broken) {
		t.Fatalf("an open beside a record that does not read: %v", err)
	}
}

// The ids rewake draws have their shapes: a space's 32 hex digits, the
// worker's session a random UUID, as the harness's --session-id takes it.
func TestDrawnIDsHaveTheirShapes(t *testing.T) {
	for range 64 {
		id, err := NewID()
		if err != nil || !ValidID(id) {
			t.Fatalf("space id %q, %v", id, err)
		}
		session, err := NewSessionID()
		if err != nil || !sessionIDShape.MatchString(session) {
			t.Fatalf("session id %q, %v", session, err)
		}
	}
}
