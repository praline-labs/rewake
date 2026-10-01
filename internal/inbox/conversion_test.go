package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
	"github.com/praline-labs/rewake/internal/state"
)

// earlierRun names the ended run of the earlier build that wrote the
// receipts: an epoch without a boot.
const earlierRun = "4194000.7"

// conversionLab is web and a main, lead, as runs of this build, and api's
// earlier run owing web what receipts are then written about.
type conversionLab struct {
	dir string
	web registry.Session
}

func newConversionLab(t *testing.T) conversionLab {
	t.Helper()
	dir := stateDir(t)
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	var web registry.Session
	for _, name := range []string{"web", "lead"} {
		session := registry.Session{Name: name, ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}
		if name == "lead" {
			session.Role = "main"
		}
		if err := registry.Publish(dir, session); err != nil {
			t.Fatal(err)
		}
		if name == "web" {
			web = session
		}
	}
	return conversionLab{dir: dir, web: web}
}

// owe records that api's earlier run read the tasks from web.
func (l conversionLab) owe(t *testing.T, tasks ...string) {
	t.Helper()
	for _, task := range tasks {
		if err := markAwaiting(l.dir, "api", earlierRun, "web", l.web.Epoch(), task); err != nil {
			t.Fatal(err)
		}
	}
}

// report is a report of api's earlier run to web answering tasks.
func (l conversionLab) report(kind Kind, tasks ...string) Message {
	return Message{ID: NewID(), From: "api", FromEpoch: earlierRun, To: "web", ToEpoch: l.web.Epoch(), Kind: kind, Text: "EARLIER_BUILD_TEXT", InReplyTo: tasks, CreatedAt: time.Now()}
}

// receipt leaves what the earlier build left for one turn end, prepared, with
// the waits api's earlier run owes now.
func (l conversionLab) receipt(t *testing.T, done bool, keep bool, reports ...Message) string {
	t.Helper()
	waiters, err := ReadWaiters(l.dir, "api", earlierRun)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"ID": NewID(), "Done": done, "Prepared": true, "KeepWaiters": keep, "Waiters": waiters, "Reports": reports})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.EnsureSubdir(TurnsPath(l.dir, "api")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(TurnsPath(l.dir, "api"), NewID())
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (l conversionLab) reconcile(t *testing.T) error {
	t.Helper()
	return state.WithMailboxLock(context.Background(), l.dir, "api", func() error {
		return Reconcile(context.Background(), l.dir, "api")
	})
}

func (l conversionLab) settle(t *testing.T, report string, delivered bool) (bool, error) {
	t.Helper()
	var recorded bool
	err := state.WithMailboxLock(context.Background(), l.dir, "api", func() error {
		var err error
		recorded, err = Settle(context.Background(), l.dir, "api", report, delivered)
		return err
	})
	return recorded, err
}

// copies counts web's letters with id.
func (l conversionLab) copies(t *testing.T, id string) int {
	t.Helper()
	letters, err := list(l.dir, "web")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, letter := range letters {
		if letter.ID == id {
			count++
		}
	}
	return count
}

func (l conversionLab) owes(t *testing.T, task string) bool {
	t.Helper()
	waiters, err := ReadWaiters(l.dir, "api", earlierRun)
	if err != nil {
		t.Fatal(err)
	}
	return slices.ContainsFunc(waiters, func(w Waiter) bool { return slices.Contains(w.Messages, task) })
}

// notes counts main's notes about api's stop.
func (l conversionLab) notes(t *testing.T) int {
	t.Helper()
	letters, err := list(l.dir, "lead")
	if err != nil {
		t.Fatal(err)
	}
	return len(letters)
}

// A prepared receipt whose report has no letter and no mark proves nothing:
// nothing is published and no wait cleared; the mailbox stops naming the
// journal and the report, and main is told once
// (docs/turn-end-recovery-findings.md#what-the-earlier-probes-now-expect).
func TestAnUnknownEarlierReportStopsTheMailbox(t *testing.T) {
	lab := newConversionLab(t)
	lab.owe(t, "t1")
	report := lab.report(Finished, "t1")
	path := lab.receipt(t, false, false, report)
	for range 2 {
		var stop *StoppedError
		err := lab.reconcile(t)
		if !errors.As(err, &stop) || len(stop.Reports) != 1 || stop.Reports[0].ID != report.ID {
			t.Fatalf("not stopped on the report: %v", err)
		}
		if !strings.Contains(err.Error(), conversionPath(lab.dir, "api")) || !strings.Contains(err.Error(), "rewake settle api "+report.ID+" --undelivered") {
			t.Fatalf("the stop does not name the journal and the way out: %v", err)
		}
	}
	if lab.copies(t, report.ID) != 0 || !lab.owes(t, "t1") {
		t.Fatal("a stopped mailbox changed")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the converted receipt stayed: %v", err)
	}
	if lab.notes(t) != 1 {
		t.Fatalf("main got %d notes, want one", lab.notes(t))
	}
	if MailboxStopped(lab.dir, "api") == nil {
		t.Fatal("the mailbox does not say it is stopped")
	}
}

// Settled undelivered, the report goes once; settled delivered, nothing goes
// and what it answered counts as answered. The same words again change
// nothing, and the opposite words are refused.
func TestSettleDecidesTheReport(t *testing.T) {
	for _, delivered := range []bool{false, true} {
		t.Run(settledWord(delivered), func(t *testing.T) {
			lab := newConversionLab(t)
			lab.owe(t, "t1")
			report := lab.report(Finished, "t1")
			lab.receipt(t, false, false, report)
			if lab.reconcile(t) == nil {
				t.Fatal("not stopped")
			}
			if recorded, err := lab.settle(t, report.ID, delivered); err != nil || !recorded {
				t.Fatalf("settle: %v %v", recorded, err)
			}
			want := map[bool]int{false: 1, true: 0}[delivered]
			if lab.copies(t, report.ID) != want || lab.owes(t, "t1") {
				t.Fatalf("copies %d, want %d; still owed %v", lab.copies(t, report.ID), want, lab.owes(t, "t1"))
			}
			if recorded, err := lab.settle(t, report.ID, delivered); err != nil || recorded {
				t.Fatalf("the same words again: %v %v", recorded, err)
			}
			var refusal *SettleRefusal
			if _, err := lab.settle(t, report.ID, !delivered); !errors.As(err, &refusal) {
				t.Fatalf("the opposite words: %v", err)
			}
			if lab.copies(t, report.ID) != want || MailboxStopped(lab.dir, "api") != nil || lab.reconcile(t) != nil {
				t.Fatal("the mailbox did not go on as settled")
			}
		})
	}
	t.Run("unknown to rewake", func(t *testing.T) {
		lab := newConversionLab(t)
		var refusal *SettleRefusal
		if _, err := lab.settle(t, NewID(), true); !errors.As(err, &refusal) {
			t.Fatalf("a report no journal names: %v", err)
		}
	})
}

// A done mark, a letter or a published mark proves the report out: it is
// completed without a second copy, and its waits are cleared. The done mark
// proves it on its own, once the recipient has read the letter and swept it.
func TestAProvenEarlierReportIsNotSentAgain(t *testing.T) {
	for _, proof := range []string{"done", "done and swept", "letter", "mark"} {
		t.Run(proof, func(t *testing.T) {
			lab := newConversionLab(t)
			lab.owe(t, "t1")
			report := lab.report(Finished, "t1")
			if proof == "done and swept" {
				lab.receipt(t, true, false, report)
				if err := lab.reconcile(t); err != nil || lab.copies(t, report.ID) != 0 || lab.owes(t, "t1") {
					t.Fatalf("a done receipt without its letter: %v, copies %d, owed %v", err, lab.copies(t, report.ID), lab.owes(t, "t1"))
				}
				return
			}
			if proof != "mark" {
				if err := PutOnce(lab.dir, report); err != nil {
					t.Fatal(err)
				}
			} else {
				path, _ := oncePath(lab.dir, "web", report.ToEpoch, report.ID)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(oncePublished), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			lab.receipt(t, proof == "done", false, report)
			if err := lab.reconcile(t); err != nil {
				t.Fatal(err)
			}
			want := map[string]int{"done": 1, "letter": 1, "mark": 0}[proof]
			if lab.copies(t, report.ID) != want || lab.owes(t, "t1") {
				t.Fatalf("copies %d, want %d; still owed %v", lab.copies(t, report.ID), want, lab.owes(t, "t1"))
			}
		})
	}
}

// A letter found is recorded in the conversion journal before anything else,
// since the recipient may sweep it before the mailbox is settled: once
// swept, the report stays proven and the settle of another goes through.
func TestAFoundLetterIsRecordedBeforeTheSweep(t *testing.T) {
	lab := newConversionLab(t)
	lab.owe(t, "a", "b")
	found, unknown := lab.report(Finished, "a"), lab.report(Finished, "b")
	if err := PutOnce(lab.dir, found); err != nil {
		t.Fatal(err)
	}
	lab.receipt(t, false, false, found)
	lab.receipt(t, false, false, unknown)
	var stop *StoppedError
	if err := lab.reconcile(t); !errors.As(err, &stop) || len(stop.Reports) != 1 || stop.Reports[0].ID != unknown.ID {
		t.Fatalf("not stopped on the unknown report alone: %v", err)
	}
	if err := archive(lab.dir, "web", found.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(state.DonePath(lab.dir, "web"), found.ID+".json")); err != nil {
		t.Fatal(err)
	}
	if _, err := lab.settle(t, unknown.ID, false); err != nil {
		t.Fatalf("the swept report stopped the mailbox again: %v", err)
	}
	if lab.copies(t, found.ID) != 0 || lab.copies(t, unknown.ID) != 1 || lab.owes(t, "a") || lab.owes(t, "b") {
		t.Fatalf("copies found %d, unknown %d; owed a %v, b %v", lab.copies(t, found.ID), lab.copies(t, unknown.ID), lab.owes(t, "a"), lab.owes(t, "b"))
	}
}

// Obligations are closed by proven reports before the rest are decided: a
// report whose every task another closed is superseded; one that closes
// nothing is withheld for good; one partly closed stops the mailbox while
// unknown, and once settled undelivered is not sent, the rest staying owed.
func TestReportsAreDecidedTogether(t *testing.T) {
	t.Run("superseded", func(t *testing.T) {
		lab := newConversionLab(t)
		lab.owe(t, "a")
		first, second := lab.report(Finished, "a"), lab.report(Finished, "a")
		lab.receipt(t, false, false, first)
		lab.receipt(t, true, false, second)
		if err := PutOnce(lab.dir, second); err != nil {
			t.Fatal(err)
		}
		if err := lab.reconcile(t); err != nil {
			t.Fatal(err)
		}
		if lab.copies(t, first.ID) != 0 || lab.owes(t, "a") {
			t.Fatal("the superseded report went, or the task stayed owed")
		}
	})
	t.Run("withheld", func(t *testing.T) {
		lab := newConversionLab(t)
		lab.owe(t, "a")
		stopped := lab.report(Stopped, "a")
		lab.receipt(t, false, true, stopped)
		if err := lab.reconcile(t); err != nil {
			t.Fatal(err)
		}
		if lab.copies(t, stopped.ID) != 0 || !lab.owes(t, "a") {
			t.Fatal("the withheld report went, or its waits were cleared")
		}
	})
	t.Run("partly closed", func(t *testing.T) {
		lab := newConversionLab(t)
		lab.owe(t, "a", "b")
		both, alone := lab.report(Finished, "a", "b"), lab.report(Finished, "a")
		lab.receipt(t, false, false, both)
		lab.receipt(t, true, false, alone)
		var stop *StoppedError
		if err := lab.reconcile(t); !errors.As(err, &stop) || len(stop.Reports) != 1 || stop.Reports[0].ID != both.ID {
			t.Fatalf("not stopped on the partly closed report: %v", err)
		}
		if _, err := lab.settle(t, both.ID, false); err != nil {
			t.Fatal(err)
		}
		if lab.copies(t, both.ID) != 0 || lab.owes(t, "a") || !lab.owes(t, "b") {
			t.Fatalf("copies %d; owed a %v, b %v", lab.copies(t, both.ID), lab.owes(t, "a"), lab.owes(t, "b"))
		}
	})
}

// A receipt that cannot be read stops the mailbox and stays; one that appears
// once the conversion is on record came from a writer the launch did not see,
// and stops it as well.
func TestAReceiptThatCannotBeConvertedStops(t *testing.T) {
	lab := newConversionLab(t)
	path := filepath.Join(TurnsPath(lab.dir, "api"), NewID())
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if lab.reconcile(t) == nil {
		t.Fatal("an unreadable receipt was passed over")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the unreadable receipt went: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	lab.receipt(t, true, false)
	if err := lab.reconcile(t); err != nil {
		t.Fatal(err)
	}
	late := lab.receipt(t, true, false)
	if lab.reconcile(t) == nil {
		t.Fatal("a receipt after the conversion was taken in")
	}
	if _, err := os.Stat(late); err != nil {
		t.Fatalf("the late receipt went: %v", err)
	}
}
