package mail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// Every strict reader of a decision's records takes one JSON value and
// nothing after it but JSON's whitespace: a closing delimiter or a token past
// the record is malformed input, never the record before it. A decision so
// damaged authorizes nothing: the barrier stops on it, and the journal it
// would close stays as it was.
func TestDecisionRecordsRefuseBytesAfterTheirValue(t *testing.T) {
	lab := newDecisionLab(t)
	d := lab.decision(t, ChoiceNotArrived)
	path := DecisionPath(lab.dir, "api", lab.report.ID)
	note := d.Notes[0].Note
	records := map[string]struct {
		raw   any
		parse func([]byte) error
	}{
		"decision": {d, func(raw []byte) error { _, err := parseDecision(path, raw); return err }},
		"closure": {
			Closure{Decision: d.ID, Journal: "end", Report: lab.report.ID, Waits: []Waiter{}, At: time.Now()},
			func(raw []byte) error { _, err := parseClosure(path+closedSuffix, raw); return err },
		},
		"told": {
			ToldRecord{Name: note.To, Run: note.ToEpoch, Note: note.ID, Answer: ToldWritten, At: time.Now()},
			func(raw []byte) error { _, err := parseTold(path+toldRecipientSuffix, raw); return err },
		},
		"resend in the decision": {
			d.Resend,
			func(raw []byte) error { e := d; e.Resend = raw; _, err := e.resendJournal(); return err },
		},
	}
	for name, record := range records {
		raw, err := json.Marshal(record.raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := record.parse(append(raw, " \t\r\n"...)); err != nil {
			t.Errorf("the %s followed by whitespace does not read: %v", name, err)
		}
		for _, suffix := range []string{"}", "]", "}garbage", "x", "{}", "null", "\v", "\f"} {
			if record.parse(append(raw, suffix...)) == nil {
				t.Errorf("the %s followed by %q reads", name, suffix)
			}
		}
	}

	for _, suffix := range []string{"}", "]", "}garbage"} {
		t.Run(suffix, func(t *testing.T) {
			lab := newDecisionLab(t)
			d := lab.decision(t, ChoiceArrived)
			raw, _ := json.Marshal(d)
			writeDecisionBytes(t, lab.dir, "api", lab.report.ID, append(raw, suffix...))
			if err := finishAll(t, lab.dir); err == nil {
				t.Fatalf("a decision followed by %q let the barrier through", suffix)
			}
			if fileExists(filepath.Join(JournalPath(lab.dir, "api"), "end"+doneSuffix)) ||
				fileExists(DecisionPath(lab.dir, "api", lab.report.ID)+closedSuffix) {
				t.Fatalf("a decision followed by %q closed its journal", suffix)
			}
			if waits, err := ReadWaiters(lab.dir, "api", lab.epoch); err != nil || len(waits) == 0 {
				t.Fatalf("a decision followed by %q cleared the waits: %v %v", suffix, waits, err)
			}
		})
	}
}

// A listing that lists nothing is a listing: the capture keeps it, and it
// survives the decision's encoding as one, apart from a listing that says
// nothing about its entries, which no capture makes.
func TestDecisionKeepsAnEmptyListingThroughItsEncoding(t *testing.T) {
	lab := newDecisionLab(t)
	d := lab.decision(t, ChoiceArrived)
	unread := filepath.Join(state.InboxPath(lab.dir, "api"), "unread")
	if err := state.EnsureSubdir(unread); err != nil {
		t.Fatal(err)
	}
	files := newCapture(osAccess{}, lab.dir)
	if _, err := files.ReadDir(unread); err != nil {
		t.Fatal(err)
	}
	observations, err := files.Observations()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkObservations(observations); err != nil {
		t.Fatalf("an empty listing does not hold together: %v", err)
	}
	d.Observations = append(d.Observations, observations...)
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	read, err := parseDecision(DecisionPath(lab.dir, "api", lab.report.ID), raw)
	if err != nil {
		t.Fatalf("a decision keeping an empty listing does not read: %v", err)
	}
	if listing := read.Observations[len(read.Observations)-1]; listing.Op != observedList || listing.Entries == nil || len(listing.Entries) != 0 {
		t.Fatalf("the empty listing came back as %+v", listing)
	}
	for name, edit := range map[string]func(*Observation){
		"a listing with no entries":  func(o *Observation) { o.Entries = nil },
		"a read with an empty entry": func(o *Observation) { *o = d.Observations[0]; o.Entries = []ObservedEntry{} },
	} {
		e := d
		e.Observations = append([]Observation(nil), d.Observations...)
		edit(&e.Observations[len(e.Observations)-1])
		raw, _ := json.Marshal(e)
		if _, err := parseDecision(DecisionPath(lab.dir, "api", lab.report.ID), raw); err == nil {
			t.Errorf("%s reads", name)
		}
	}
}

// What the capture hands out is a copy down to its metadata: a caller that
// changes an observation's stat, error or lstat changes nothing kept.
func TestDecisionObservationsShareNothingWithTheCapture(t *testing.T) {
	dir := stateDir(t)
	box := state.InboxPath(dir, "api")
	if err := state.EnsureSubdir(box); err != nil {
		t.Fatal(err)
	}
	files := newCapture(osAccess{}, dir)
	if _, err := files.Stat(box); err != nil {
		t.Fatal(err)
	}
	if _, err := files.ReadFile(filepath.Join(box, "gone.json")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	first, _ := files.Observations()
	first[0].Info.Size = -1
	first[1].Err.Text = "changed"
	first[1].Lstat.Err.Kind = "changed"
	again, _ := files.Observations()
	if again[0].Info.Size == -1 || again[1].Err.Text == "changed" || again[1].Lstat.Err.Kind == "changed" {
		t.Fatalf("the observations handed out share metadata with the capture: %+v", again)
	}
}
