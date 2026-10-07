package inbox

import (
	"os"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
)

// The kept answer belongs to one run: another run of the name reads none, and
// only the version a journal names is taken.
func TestAKeptAnswerBelongsToItsRun(t *testing.T) {
	dir := stateDir(t)
	if err := KeepAnswer(dir, "api", "1.1", "the answer", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := KeptAnswer(dir, "api", "2.2"); ok {
		t.Error("another run read this run's kept answer")
	}
	taken, ok, err := KeptAnswerThrough(dir, "api", "1.1", nil)
	version := taken.Version
	if err != nil || !ok {
		t.Fatalf("the answer: %v %v", ok, err)
	}
	if err := live(dir).dropKeptVersion("api", "1.1", "an earlier version"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := KeptAnswer(dir, "api", "1.1"); !ok {
		t.Error("dropping another version took this answer")
	}
	if err := live(dir).dropKeptVersion("api", "1.1", version); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := KeptAnswer(dir, "api", "1.1"); ok {
		t.Error("the answer outlived dropping its version")
	}
	if err := live(dir).dropKeptVersion("api", "1.1", version); err != nil {
		t.Errorf("dropping nothing: %v", err)
	}
}

// A hold takes its place on the read clock: an end with a boundary captured
// before the hold does not take the answer, one captured after does, and an
// end heard once takes it as it finds it (docs/turn-end-recovery.md#the-read-clock).
func TestAKeptAnswerIsTakenOnlyAtOrBelowTheBoundary(t *testing.T) {
	dir := stateDir(t)
	if err := KeepAnswer(dir, "api", "1.1", "held", "", ""); err != nil {
		t.Fatal(err)
	}
	record, _, err := readKept(dir, "api", "1.1")
	if err != nil || record.Seq == 0 {
		t.Fatalf("the hold took no place on the clock: %+v %v", record, err)
	}
	for _, c := range []struct {
		through *uint64
		want    bool
	}{{ptr(record.Seq - 1), false}, {ptr(record.Seq), true}, {ptr(record.Seq + 5), true}, {nil, true}} {
		if _, ok, err := KeptAnswerThrough(dir, "api", "1.1", c.through); err != nil || ok != c.want {
			t.Errorf("through %v: %v %v, want %v", c.through, ok, err, c.want)
		}
	}
	if err := KeepAnswer(dir, "api", "1.1", "held again", "", ""); err != nil {
		t.Fatal(err)
	}
	if next, _, _ := readKept(dir, "api", "1.1"); next.Seq <= record.Seq {
		t.Errorf("a later hold took position %d after %d", next.Seq, record.Seq)
	}
}

// A kept answer that cannot be read is never written over: it may be one no
// end has published yet.
func TestAnUnreadableKeptAnswerIsNotWrittenOver(t *testing.T) {
	dir := stateDir(t)
	if err := KeepAnswer(dir, "api", "1.1", "held", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keptPath(dir, "api"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := KeepAnswer(dir, "api", "1.1", "another", "", ""); err == nil {
		t.Error("an unreadable kept answer was written over")
	}
	if _, _, err := KeptAnswerThrough(dir, "api", "1.1", nil); err == nil {
		t.Error("an unreadable kept answer read as none")
	}
}

func ptr(value uint64) *uint64 { return &value }

// ownedRun publishes api as a run of this build and answers its epoch.
func ownedRun(t *testing.T, dir string) string {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	api := registry.Session{Name: "api", ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}
	if err := registry.Publish(dir, api); err != nil {
		t.Fatal(err)
	}
	return api.Epoch()
}

func interimNow(t *testing.T, dir string) *interimRecord {
	t.Helper()
	record, err := readInterim(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func line(text string) *string { return &text }

// The interim table (docs/turn-end-recovery.md#the-interim-record): a record
// of this run is written over only by an end that ended later, a retry of the
// same operation is done, and a tie is fixed whatever the order of retries.
func TestTheInterimRecordKeepsTheLastWord(t *testing.T) {
	dir := stateDir(t)
	epoch := ownedRun(t, dir)
	if err := recordInterim(dir, "api", epoch, "b", 200, line("waits"), false); err != nil {
		t.Fatal(err)
	}
	if text, ok, _ := LastInterim(dir, "api", epoch); !ok || text != "waits" {
		t.Fatalf("absent, then written: %q %v", text, ok)
	}
	// A settling end that ended earlier is late: superseded.
	if err := recordInterim(dir, "api", epoch, "a", 150, nil, true); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := LastInterim(dir, "api", epoch); !ok {
		t.Error("a late settling end brought down a later interim end")
	}
	// The same operation again is its retry: done.
	if err := recordInterim(dir, "api", epoch, "b", 200, line("other words"), false); err != nil {
		t.Fatal(err)
	}
	if text, _, _ := LastInterim(dir, "api", epoch); text != "waits" {
		t.Errorf("a retry wrote again: %q", text)
	}
	// Ended later: written over, and settled stays settled for a late one.
	if err := recordInterim(dir, "api", epoch, "c", 300, nil, true); err != nil {
		t.Fatal(err)
	}
	if err := recordInterim(dir, "api", epoch, "d", 250, line("late"), false); err != nil {
		t.Fatal(err)
	}
	if record := interimNow(t, dir); !record.Settled || record.Op != "c" {
		t.Errorf("a late interim end brought back a line settled after it: %+v", record)
	}
	// A stop records nothing.
	if err := recordInterim(dir, "api", epoch, "e", 400, nil, false); err != nil {
		t.Fatal(err)
	}
	if record := interimNow(t, dir); record.Op != "c" {
		t.Errorf("a stop recorded a word: %+v", record)
	}
}

// Two ends of one time are ordered without regard to the order they come in:
// interim over settled, then the operation whose name sorts last.
func TestAnInterimTieDoesNotDependOnOrder(t *testing.T) {
	type end struct {
		op   string
		line *string
	}
	for _, c := range []struct {
		name    string
		ends    []end
		op      string
		interim bool
	}{
		{"interim beats settled", []end{{"z", nil}, {"a", line("waits")}}, "a", true},
		{"between interims the last name", []end{{"a", line("one")}, {"b", line("two")}}, "b", true},
		{"between settled the last name", []end{{"a", nil}, {"b", nil}}, "b", false},
	} {
		for _, reverse := range []bool{false, true} {
			dir := stateDir(t)
			epoch := ownedRun(t, dir)
			ends := c.ends
			if reverse {
				ends = []end{ends[1], ends[0]}
			}
			for _, e := range ends {
				if err := recordInterim(dir, "api", epoch, e.op, 100, e.line, e.line == nil); err != nil {
					t.Fatal(err)
				}
			}
			if record := interimNow(t, dir); record.Op != c.op || record.Settled == c.interim {
				t.Errorf("%s, reversed %v: %+v", c.name, reverse, record)
			}
		}
	}
}

// A record of another run is written over by the run holding the name, and
// left alone by a journal of a run that no longer holds it. One that cannot
// be read stops the end, naming it.
func TestTheInterimRecordOfAnotherRun(t *testing.T) {
	dir := stateDir(t)
	epoch := ownedRun(t, dir)
	if err := recordInterim(dir, "api", "ended.1", "x", 900, line("old"), false); err != nil {
		t.Fatal(err)
	}
	if record := interimNow(t, dir); record.Epoch != "ended.1" {
		t.Fatalf("absent, then written by any run: %+v", record)
	}
	if err := recordInterim(dir, "api", epoch, "y", 10, nil, true); err != nil {
		t.Fatal(err)
	}
	if record := interimNow(t, dir); record.Epoch != epoch || !record.Settled {
		t.Errorf("the run holding the name did not write over another run's: %+v", record)
	}
	if err := recordInterim(dir, "api", "ended.1", "z", 999, line("late"), false); err != nil {
		t.Fatal(err)
	}
	if record := interimNow(t, dir); record.Epoch != epoch {
		t.Errorf("an ended run's journal wrote over the live run's: %+v", record)
	}
	if err := os.WriteFile(interimPath(dir, "api"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := recordInterim(dir, "api", epoch, "w", 1000, nil, true); err == nil {
		t.Error("an unreadable interim record was written over")
	}
	if _, _, err := LastInterim(dir, "api", epoch); err == nil {
		t.Error("an unreadable interim record read as none")
	}
}
