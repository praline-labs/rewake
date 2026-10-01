package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
	"github.com/praline-labs/rewake/internal/state"
)

// Every durable stop outlives a barrier that did not run its effects
// through: canceled, stopped by another unknown the reading found and that
// then left, or failing on the effect itself. Only a barrier that ran every
// effect lifts a stop an effect met, or one whose record does not read; one
// the reading found stands while its cause does (stop.go).
func TestEveryDurableStopOutlivesAFailedRetry(t *testing.T) {
	for _, stop := range []string{"effect", "record", "reading"} {
		for _, retry := range []string{"canceled", "another stop", "failing effect"} {
			t.Run(stop+"/"+retry, func(t *testing.T) {
				lab := newConversionLab(t)
				version := "v1"
				raw, err := json.Marshal(keptRecord{Epoch: earlierRun, Text: "held", Version: version})
				if err != nil {
					t.Fatal(err)
				}
				kept := keptPath(lab.dir, "api")
				writeRaw(t, kept, string(raw))
				if err := WriteJournal(lab.dir, "api", "end", TurnJournal{Epoch: earlierRun, Op: "end", Kept: &version}); err != nil {
					t.Fatal(err)
				}
				cause := filepath.Join(JournalPath(lab.dir, "api"), "z")
				switch stop {
				case "effect":
					t.Cleanup(func() { afterReading, afterEffect = func() {}, func() {} })
					afterReading = func() { _ = os.Chmod(kept, 0) }
					afterEffect = func() { _ = os.Chmod(kept, 0o600) }
					var unknown *UnknownRecordError
					if err := lab.reconcile(t); !errors.As(err, &unknown) {
						t.Fatalf("the effect did not meet the unknown: %v", err)
					}
					afterReading, afterEffect = func() {}, func() {}
				case "record":
					writeRaw(t, stopPath(lab.dir, "api"), "{")
				case "reading":
					writeRaw(t, cause, "{")
					if lab.reconcile(t) == nil {
						t.Fatal("an unreadable journal went through")
					}
				}
				switch retry {
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					if withLock(lab.dir, "api", func() error { return Reconcile(ctx, lab.dir, "api") }) == nil {
						t.Fatal("a canceled barrier went through")
					}
				case "another stop":
					other := interimPath(lab.dir, "api")
					writeRaw(t, other, "{")
					if lab.reconcile(t) == nil {
						t.Fatal("the barrier went on past an unreadable interim record")
					}
					if err := os.Remove(other); err != nil {
						t.Fatal(err)
					}
				case "failing effect":
					pending := filepath.Dir(kept)
					if err := os.Chmod(pending, 0o500); err != nil {
						t.Fatal(err)
					}
					err := lab.reconcile(t)
					_ = os.Chmod(pending, 0o700)
					if err == nil {
						t.Fatal("the kept answer was taken from a directory closed to writes")
					}
				}
				if _, err := os.Stat(kept); err != nil {
					t.Fatalf("the effect ran through: %v", err)
				}
				if MailboxStopped(lab.dir, "api") == nil {
					t.Fatalf("a %s retry lifted the %s stop without running its effect", retry, stop)
				}
				if stop == "reading" {
					if err := os.Remove(cause); err != nil {
						t.Fatal(err)
					}
				}
				if err := lab.reconcile(t); err != nil {
					t.Fatalf("the barrier that runs the effect through: %v", err)
				}
				if _, err := os.Stat(kept); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("the kept answer stayed: %v", err)
				}
				if err := MailboxStopped(lab.dir, "api"); err != nil {
					t.Fatalf("the stop outlived the barrier that ran its effect: %v", err)
				}
			})
		}
	}
}

// Every mark an effect of the barrier will decide by is read by the reading
// every gate and the barrier make, before the first effect, and a path to it
// that cannot be followed — a file where its
// directory belongs, or a directory closed to reading — is an unknown, not
// an absence (rule 6): the report of an earlier journal is not published
// beside it, and the mailbox stays stopped.
func TestEveryEvidencePathIsReadBeforeTheFirstEffect(t *testing.T) {
	for _, path := range []string{"recipient", "recorded successor", "chosen successor", "owed note", "earlier receipt", "conversion"} {
		for _, fault := range []string{"not a directory", "no access"} {
			t.Run(path+"/"+fault, func(t *testing.T) {
				lab := newConversionLab(t)
				lead, err := registry.Load(lab.dir, "lead")
				if err != nil {
					t.Fatal(err)
				}
				sender, first, to, epoch := "web", Message{}, "lead", lead.Epoch()
				earlierJournal := func(reports ...Message) {
					if err := WriteJournal(lab.dir, "web", "a", TurnJournal{Epoch: lab.web.Epoch(), Op: "end", Reports: reports}); err != nil {
						t.Fatal(err)
					}
				}
				later := func(journal TurnJournal) {
					journal.Epoch, journal.Op = lab.web.Epoch(), "end"
					if err := WriteJournal(lab.dir, "web", "b", journal); err != nil {
						t.Fatal(err)
					}
				}
				held := Message{ID: NewID(), From: "web", FromEpoch: lab.web.Epoch(), To: "api", ToEpoch: earlierRun, Kind: Finished, Text: "HELD", CreatedAt: time.Now()}
				switch path {
				case "recipient", "recorded successor", "chosen successor", "owed note":
					first = Message{ID: NewID(), From: "web", FromEpoch: lab.web.Epoch(), To: "ops", ToEpoch: lab.session(t, "ops").Epoch(), Kind: Finished, Text: "FIRST", CreatedAt: time.Now()}
					earlierJournal(first)
				default:
					sender, first = "api", lab.report(Finished, "one")
				}
				second := lab.report(Finished, "two")
				second.To, second.ToEpoch = "lead", lead.Epoch()
				switch path {
				case "recipient":
					second.From, second.FromEpoch = "web", lab.web.Epoch()
					later(TurnJournal{Reports: []Message{second}})
				case "recorded successor", "chosen successor":
					start, err := proc.StartTime(os.Getpid())
					if err != nil {
						t.Fatal(err)
					}
					successor := lab.bind(t, os.Getpid(), start)
					if err := registry.Publish(lab.dir, successor); err != nil {
						t.Fatal(err)
					}
					to, epoch = "api", successor.Epoch()
					journal := TurnJournal{Reports: []Message{held}}
					if path == "recorded successor" {
						journal.Held, journal.Successors = []string{held.ID}, map[string]string{held.ID: epoch}
					}
					later(journal)
				case "owed note":
					later(TurnJournal{Reports: []Message{held}, Moot: []string{held.ID}, Notices: []string{held.ID}})
				case "earlier receipt", "conversion":
					lab.owe(t, "one", "two")
					lab.receipt(t, false, false, first, second)
					if path == "conversion" {
						receipts, err := earlierReceipts(lab.dir, "api")
						if err != nil {
							t.Fatal(err)
						}
						if err := (&conversionJournal{Receipts: receipts}).save(lab.dir, "api"); err != nil {
							t.Fatal(err)
						}
					}
				}
				blocked := filepath.Join(state.InboxPath(lab.dir, to), "once", epoch)
				t.Cleanup(func() { _ = os.Chmod(blocked, 0o700); _ = os.RemoveAll(blocked) })
				if fault == "not a directory" {
					writeRaw(t, blocked, "no directory")
				} else {
					if err := os.MkdirAll(blocked, 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(blocked, 0); err != nil {
						t.Fatal(err)
					}
				}
				// A gate asked before any barrier ran reads the same marks: a
				// pending or a read goes on only past a mailbox the reading
				// found whole, not one some effect has yet to trip on.
				if MailboxStopped(lab.dir, sender) == nil {
					t.Fatalf("the reading does not read the %s's mark", path)
				}
				err = withLock(lab.dir, sender, func() error { return Reconcile(context.Background(), lab.dir, sender) })
				if err == nil {
					t.Fatalf("the barrier went on past %s at the %s's mark", fault, path)
				}
				t.Logf("barrier: %v", err)
				if found, err := present(lab.dir, first.To, first.ID); err != nil || found {
					t.Fatalf("the first report went out before the %s's mark was read: %v %v", path, found, err)
				}
				if MailboxStopped(lab.dir, sender) == nil {
					t.Fatalf("%s at the %s's mark left the mailbox open", fault, path)
				}
			})
		}
	}
}

// session publishes a running session of this build under name.
func (l conversionLab) session(t *testing.T, name string) registry.Session {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	session := registry.Session{Name: name, ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: l.dir, StartedAt: time.Now()}
	if err := registry.Publish(l.dir, session); err != nil {
		t.Fatal(err)
	}
	return session
}

// heldMootNote is the note tellMainHeldMoot leaves for report, looked up
// without removing any record.
func heldMootNote(dir, from, report string) (Message, bool, error) {
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return Message{}, false, err
	}
	note, ok := mainNote(sessions, from, heldMootAbout+report, "")
	return note, ok, nil
}
