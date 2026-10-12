package mail

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// planScene is a state the barrier of one mailbox runs on, built as the
// tests of each effect build it.
type planScene struct {
	name  string
	build func(t *testing.T, lab twoSessionLab) (mailbox string)
}

var planScenes = []planScene{
	{"two journals to two recipients", func(t *testing.T, lab twoSessionLab) string {
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
	{"a report recorded moot", func(t *testing.T, lab twoSessionLab) string {
		moot := Message{ID: NewID(), From: "web", FromEpoch: lab.web.Epoch(), To: "api", ToEpoch: lab.run, Kind: Finished, Text: "MOOT", CreatedAt: time.Now()}
		lab.journal(t, "end", TurnJournal{Reports: []Message{moot}, Moot: []string{moot.ID}})
		return "web"
	}},
	{"the steps of a journal", func(t *testing.T, lab twoSessionLab) string {
		lab.owe(t, "t1")
		waiters, err := ReadWaiters(lab.dir, "api", lab.run)
		if err != nil {
			t.Fatal(err)
		}
		version, interim := "v1", "still going"
		raw, err := json.Marshal(keptRecord{Epoch: lab.run, Text: "held", Version: version})
		if err != nil {
			t.Fatal(err)
		}
		writeRaw(t, keptPath(lab.dir, "api"), string(raw))
		report := lab.report(Finished, "t1")
		if err := WriteJournal(lab.dir, "api", "end", TurnJournal{Epoch: lab.run, Op: "end", Ended: 200, Reports: []Message{report}, Clear: waiters, Kept: &version, Interim: &interim}); err != nil {
			t.Fatal(err)
		}
		return "api"
	}},
	// The plan's writes do nothing, so a later journal of the same pass reads
	// what an earlier one would have changed: two clear parts of one wait and
	// share the kept answer and the interim line.
	{"shared wait, kept and interim across journals", func(t *testing.T, lab twoSessionLab) string {
		lab.owe(t, "t1", "t2")
		waiters, err := ReadWaiters(lab.dir, "api", lab.run)
		if err != nil || len(waiters) != 1 {
			t.Fatalf("waiters %v: %v", waiters, err)
		}
		version, line := "v1", "still going"
		raw, err := json.Marshal(keptRecord{Epoch: lab.run, Text: "held", Version: version})
		if err != nil {
			t.Fatal(err)
		}
		writeRaw(t, keptPath(lab.dir, "api"), string(raw))
		for i, id := range []string{"a", "b"} {
			wait := waiters[0]
			wait.Messages = []string{[]string{"t1", "t2"}[i]}
			journal := TurnJournal{Epoch: lab.run, Op: id, Ended: int64(100 + i), Reports: []Message{lab.report(Finished, wait.Messages...)}, Clear: []Waiter{wait}, Kept: &version, Interim: &line}
			if err := WriteJournal(lab.dir, "api", id, journal); err != nil {
				t.Fatal(err)
			}
		}
		return "api"
	}},
	// The installation is the barrier's before the plan, stopped or not, and
	// a note that cannot be told stops nothing, so a fault there is no stop:
	// TestDecisionInstallationSurvivesEveryBrokenWrite holds it. Here what
	// the plan sees of the installation is read again, and the journals run.
	{"a decision installed", func(t *testing.T, lab twoSessionLab) string {
		lab.owe(t, "t1")
		waiters, err := ReadWaiters(lab.dir, "api", lab.run)
		if err != nil {
			t.Fatal(err)
		}
		report := lab.report(Finished, "t1")
		if err := WriteJournal(lab.dir, "api", "end", TurnJournal{Epoch: lab.run, Op: "end", Ended: 200, Reports: []Message{report}, Clear: waiters}); err != nil {
			t.Fatal(err)
		}
		writeDecision(t, lab.dir, newDecision(t, lab.dir, "end", lab.run, report, ChoiceNotArrived))
		if err := withLock(lab.dir, "api", func() error { return live(lab.dir).installDecisions(context.Background(), "api") }); err != nil {
			t.Fatal(err)
		}
		return "api"
	}},
	{"a record of every kind", func(t *testing.T, lab twoSessionLab) string {
		everyKind(t, lab)
		return "api"
	}},
}

// everyKind leaves in api's mailbox a file of every kind the reading opens,
// each as its writer leaves it, beside a completed journal.
func everyKind(t *testing.T, lab twoSessionLab) {
	t.Helper()
	box := state.InboxPath(lab.dir, "api")
	letter := message("sample")
	raw, err := json.Marshal(letter)
	if err != nil {
		t.Fatal(err)
	}
	lab.owe(t, "t1")
	if err := WriteJournal(lab.dir, "api", "old", TurnJournal{Epoch: lab.run, Op: "old", Ended: 100}); err != nil {
		t.Fatal(err)
	}
	if err := live(lab.dir).finishJournal(t.Context(), "api", "old"); err != nil {
		t.Fatal(err)
	}
	if err := MarkPending(lab.dir, "api", lab.run, MarkName(150, NewID()), "waiting", 150); err != nil {
		t.Fatal(err)
	}
	if err := RecordTurnStart(lab.dir, "api", lab.run, 120); err != nil {
		t.Fatal(err)
	}
	token := "0123456789abcdef01234567"
	record, err := json.Marshal(map[string]any{"version": 1, "token": token, "epoch": "e1"})
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		letter.ID + ".json":                    string(raw),
		letter.ID + ".status":                  `{"state":"delivered"}`,
		"unread/" + NewID() + ".json":          string(raw),
		"done/" + NewID() + ".json":            string(raw),
		"awaiting/" + lab.run + "/.read-clock": "\x00\x00\x00\x00\x00\x00\x00\x01",
		"awaiting/" + lab.run + "/.read-high":  "1",
		"answering/q1":                         "",
		"received/q1":                          "",
		"retention/q1":                         `{"releasedAt":"2026-01-01T00:00:00Z"}`,
		"pending/kept.json":                    `{"epoch":"` + lab.run + `","text":"held","version":"v0"}`,
		"pending/interim.json":                 `{"epoch":"` + lab.run + `","text":"going"}`,
		"once/" + lab.run + "/" + NewID():      oncePublished,
		"threads/" + letter.ID:                 "",
		"claims/" + letter.ID:                  "",
		"receipts/e1/key-call":                 token,
		"receipts/e1/" + token + ".json":       string(record),
		"receipts/e1/calls/abababababababababababababababababababababababababababababababab": token,
	} {
		writeRaw(t, filepath.Join(box, path), content)
	}
	for path, content := range decidedRecords(t, "api", lab.run, "web", lab.web.Epoch()) {
		writeRaw(t, filepath.Join(box, path), content)
	}
}
