package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// planScene is a state the barrier of one mailbox runs on, built as the
// tests of each effect build it.
type planScene struct {
	name  string
	build func(t *testing.T, lab conversionLab) (mailbox string)
}

var planScenes = []planScene{
	{"two journals to two recipients", func(t *testing.T, lab conversionLab) string {
		first := Message{ID: NewID(), From: "web", FromEpoch: lab.web.Epoch(), To: "ops", ToEpoch: lab.session(t, "ops").Epoch(), Kind: Finished, Text: "FIRST", CreatedAt: time.Now()}
		lead, err := registry.Load(lab.dir, "lead")
		if err != nil {
			t.Fatal(err)
		}
		second := Message{ID: NewID(), From: "web", FromEpoch: lab.web.Epoch(), To: "lead", ToEpoch: lead.Epoch(), Kind: Finished, Text: "SECOND", CreatedAt: time.Now()}
		lab.journal(t, "a", TurnJournal{Reports: []Message{first}})
		lab.journal(t, "b", TurnJournal{Reports: []Message{second}})
		return "web"
	}},
	{"held, successor chosen now", func(t *testing.T, lab conversionLab) string {
		lab.holdReport(t)
		lab.liveSuccessor(t)
		return "web"
	}},
	{"held, successor recorded", func(t *testing.T, lab conversionLab) string {
		lab.chooseSuccessor(t, lab.holdReport(t), lab.liveSuccessor(t))
		return "web"
	}},
	{"held, successor gone, a new note", func(t *testing.T, lab conversionLab) string {
		lab.chooseSuccessor(t, lab.holdReport(t), lab.bind(t, 4194001, 9))
		return "web"
	}},
	{"a note owed", func(t *testing.T, lab conversionLab) string {
		held := Message{ID: NewID(), From: "web", FromEpoch: lab.web.Epoch(), To: "api", ToEpoch: earlierRun, Kind: Finished, Text: "HELD", CreatedAt: time.Now()}
		lab.journal(t, "end", TurnJournal{Reports: []Message{held}, Moot: []string{held.ID}, Notices: []string{held.ID}})
		return "web"
	}},
	{"an earlier receipt done", func(t *testing.T, lab conversionLab) string {
		lab.owe(t, "t1")
		lab.receipt(t, true, false, lab.report(Finished, "t1"))
		return "api"
	}},
	{"a conversion settled undelivered", func(t *testing.T, lab conversionLab) string {
		lab.owe(t, "t1")
		report := lab.report(Finished, "t1")
		lab.receipt(t, false, false, report)
		if lab.reconcile(t) == nil {
			t.Fatal("the report was decided without a person")
		}
		conversion, err := readConversion(lab.dir, "api")
		if err != nil {
			t.Fatal(err)
		}
		conversion.Settled = map[string]bool{report.ID: false}
		if err := conversion.save(lab.dir, "api"); err != nil {
			t.Fatal(err)
		}
		return "api"
	}},
	{"a conversion that stops", func(t *testing.T, lab conversionLab) string {
		lab.owe(t, "t1")
		lab.receipt(t, false, false, lab.report(Finished, "t1"))
		return "api"
	}},
	{"the steps of a journal", func(t *testing.T, lab conversionLab) string {
		lab.owe(t, "t1")
		waiters, err := ReadWaiters(lab.dir, "api", earlierRun)
		if err != nil {
			t.Fatal(err)
		}
		version, interim := "v1", "still going"
		raw, err := json.Marshal(keptRecord{Epoch: earlierRun, Text: "held", Version: version})
		if err != nil {
			t.Fatal(err)
		}
		writeRaw(t, keptPath(lab.dir, "api"), string(raw))
		report := lab.report(Finished, "t1")
		if err := WriteJournal(lab.dir, "api", "end", TurnJournal{Epoch: earlierRun, Op: "end", Ended: 200, Reports: []Message{report}, Clear: waiters, Kept: &version, Interim: &interim}); err != nil {
			t.Fatal(err)
		}
		return "api"
	}},
	// The plan's writes do nothing, so a later journal of the same pass reads
	// what an earlier one would have changed: two clear parts of one wait and
	// share the kept answer and the interim line.
	{"shared wait, kept and interim across journals", func(t *testing.T, lab conversionLab) string {
		lab.owe(t, "t1", "t2")
		waiters, err := ReadWaiters(lab.dir, "api", earlierRun)
		if err != nil || len(waiters) != 1 {
			t.Fatalf("waiters %v: %v", waiters, err)
		}
		version, line := "v1", "still going"
		raw, err := json.Marshal(keptRecord{Epoch: earlierRun, Text: "held", Version: version})
		if err != nil {
			t.Fatal(err)
		}
		writeRaw(t, keptPath(lab.dir, "api"), string(raw))
		for i, id := range []string{"a", "b"} {
			wait := waiters[0]
			wait.Messages = []string{[]string{"t1", "t2"}[i]}
			journal := TurnJournal{Epoch: earlierRun, Op: id, Ended: int64(100 + i), Reports: []Message{lab.report(Finished, wait.Messages...)}, Clear: []Waiter{wait}, Kept: &version, Interim: &line}
			if err := WriteJournal(lab.dir, "api", id, journal); err != nil {
				t.Fatal(err)
			}
		}
		return "api"
	}},
	{"a record of every kind", func(t *testing.T, lab conversionLab) string {
		everyKind(t, lab)
		return "api"
	}},
}

// journal records an unfinished turn journal of web's under id.
func (l conversionLab) journal(t *testing.T, id string, journal TurnJournal) {
	t.Helper()
	journal.Epoch, journal.Op = l.web.Epoch(), id
	if err := WriteJournal(l.dir, "web", id, journal); err != nil {
		t.Fatal(err)
	}
}

// liveSuccessor binds api's successor to this process and publishes it, so
// it is ready to take what is held for its name.
func (l conversionLab) liveSuccessor(t *testing.T) registry.Session {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	successor := l.bind(t, os.Getpid(), start)
	if err := registry.Publish(l.dir, successor); err != nil {
		t.Fatal(err)
	}
	return successor
}

// everyKind leaves in api's mailbox a file of every kind the reading opens,
// each as its writer leaves it, beside the done receipt of an earlier build
// and a completed journal.
func everyKind(t *testing.T, lab conversionLab) {
	t.Helper()
	box := state.InboxPath(lab.dir, "api")
	letter := message("sample")
	raw, err := json.Marshal(letter)
	if err != nil {
		t.Fatal(err)
	}
	lab.owe(t, "t1")
	lab.receipt(t, true, false)
	if err := WriteJournal(lab.dir, "api", "old", TurnJournal{Epoch: earlierRun, Op: "old", Ended: 100}); err != nil {
		t.Fatal(err)
	}
	if err := live(lab.dir).finishJournal(t.Context(), "api", "old"); err != nil {
		t.Fatal(err)
	}
	if err := MarkPending(lab.dir, "api", earlierRun, MarkName(150, NewID()), "waiting", 150); err != nil {
		t.Fatal(err)
	}
	token := "0123456789abcdef01234567"
	record, err := json.Marshal(map[string]any{"version": 1, "token": token, "epoch": "e1"})
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		letter.ID + ".json":                       string(raw),
		letter.ID + ".status":                     `{"state":"delivered"}`,
		"unread/" + NewID() + ".json":             string(raw),
		"done/" + NewID() + ".json":               string(raw),
		"awaiting/" + earlierRun + "/.read-clock": "\x00\x00\x00\x00\x00\x00\x00\x01",
		"awaiting/" + earlierRun + "/.read-high":  "1",
		"answering/q1":                            "",
		"received/q1":                             "",
		"retention/q1":                            `{"releasedAt":"2026-01-01T00:00:00Z"}`,
		"pending/kept.json":                       `{"epoch":"` + earlierRun + `","text":"held","version":"v0"}`,
		"pending/interim.json":                    `{"epoch":"` + earlierRun + `","text":"going"}`,
		"once/" + earlierRun + "/" + NewID():      oncePublished,
		"threads/" + letter.ID:                    "",
		"claims/" + letter.ID:                     "",
		"receipts/e1/key-call":                    token,
		"receipts/e1/" + token + ".json":          string(record),
	} {
		writeRaw(t, filepath.Join(box, path), content)
	}
}
