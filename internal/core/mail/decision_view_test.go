package mail

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// writeResend installs d's resend as its installation will, done in place
// with summary when it is set, or renamed done as well.
func writeResend(t *testing.T, lab decisionLab, d Decision, summary *ResendSummary, renamed bool) string {
	t.Helper()
	journal, err := d.resendJournal()
	if err != nil {
		t.Fatal(err)
	}
	if summary != nil {
		journal = TurnJournal{Epoch: journal.Epoch, Op: journal.Op, Done: true, Summary: summary}
	}
	if err := WriteJournal(lab.dir, "api", ResendJournal(d.ID), journal); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(JournalPath(lab.dir, "api"), ResendJournal(d.ID))
	if renamed {
		if err := os.Rename(path, path+doneSuffix); err != nil {
			t.Fatal(err)
		}
		path += doneSuffix
	}
	return path
}

func writeRecord(t *testing.T, path string, record any) {
	t.Helper()
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	writeRaw(t, path, string(raw))
}

// A view of decisions says what each phase established and nothing more: a
// decision recorded is not a resend installed, a resend installed is not one
// published, a note owed is not told, and no closure until its tail wrote one.
// It reads and changes nothing.
func TestDecisionViewsDuringEveryPhase(t *testing.T) {
	lab := newDecisionLab(t)
	d := lab.decision(t, ChoiceNotArrived)
	path := writeDecision(t, lab.dir, d)
	look := func(phase string) DecisionView {
		t.Helper()
		before := snapshot(t, lab.dir)
		views, err := Decisions(lab.dir, "api")
		if err != nil || len(views) != 1 || views[0].Err != nil {
			t.Fatalf("%s: the decisions read %+v %v", phase, views, err)
		}
		if after := snapshot(t, lab.dir); !maps.Equal(before, after) {
			t.Fatalf("%s: a view changed the mailbox", phase)
		}
		return views[0]
	}

	view := look("recorded")
	if view.Resend.Installed || view.Closure != nil || view.Told[ToldRecipient].Record != nil || view.Decision.ID != d.ID {
		t.Fatalf("a decision just recorded shows %+v", view)
	}
	journal := writeResend(t, lab, d, nil, false)
	if view = look("installed"); !view.Resend.Installed || view.Resend.Done || view.Resend.Summary != nil {
		t.Fatalf("a resend installed shows %+v", view.Resend)
	}
	told := ToldRecord{Name: lab.report.To, Run: lab.report.ToEpoch, Note: d.Notes[0].Note.ID, Answer: ToldWritten, At: time.Now()}
	writeRecord(t, toldPath(lab.dir, "api", lab.report.ID, ToldRecipient), told)
	if view = look("told"); view.Told[ToldRecipient].Record == nil {
		t.Fatalf("a note told shows %+v", view.Told)
	}
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	resent, _ := d.resendJournal()
	summary := &ResendSummary{Report: resent.Reports[0].ID, Disposition: DispositionMoot, Decision: d.ID}
	writeResend(t, lab, d, summary, false)
	if view = look("done in place"); !view.Resend.Done || view.Resend.Summary.Disposition != DispositionMoot {
		t.Fatalf("a resend done in place shows %+v", view.Resend)
	}
	if err := os.Rename(journal, journal+doneSuffix); err != nil {
		t.Fatal(err)
	}
	if view = look("renamed"); !view.Resend.Done || view.Resend.Summary == nil {
		t.Fatalf("a resend renamed done shows %+v", view.Resend)
	}
	if view.Closure != nil {
		t.Fatal("a closure shows before its tail wrote one")
	}
	writeRecord(t, path+closedSuffix, Closure{Decision: d.ID, Journal: "end", Report: lab.report.ID, Waits: []Waiter{}, At: time.Now()})
	if view = look("closed"); view.Closure == nil || view.ClosureErr != nil {
		t.Fatalf("a closure written shows %+v %v", view.Closure, view.ClosureErr)
	}

	// What does not read is named where it stands, never taken for a phase.
	writeRaw(t, path+closedSuffix, `{"decision":"`+NewID()+`","journal":"end","report":"`+lab.report.ID+`","waits":[],"at":"2026-10-12T00:00:00Z"}`)
	writeRaw(t, toldPath(lab.dir, "api", lab.report.ID, ToldRecipient), `{`)
	if view = look("damaged"); view.Closure != nil || view.ClosureErr == nil || view.Told[ToldRecipient].Err == nil {
		t.Fatalf("records that do not read show %+v", view)
	}
	writeRaw(t, path, `{`)
	views, err := Decisions(lab.dir, "api")
	if err != nil || len(views) != 1 || views[0].Err == nil || views[0].Decision != nil {
		t.Fatalf("a decision that does not read shows %+v %v", views, err)
	}
}

// A journal a decision installed outlives every sweep, in every form and
// after its run ended, while the done journals around it go as before.
func TestDecisionEffectsSurviveAllRetention(t *testing.T) {
	lab := newDecisionLab(t)
	old := time.Now().Add(-keepFinished - time.Hour)
	forms := map[string]func(Decision) string{
		"open": func(d Decision) string { return writeResend(t, lab, d, nil, false) },
		"done in place": func(d Decision) string {
			return writeResend(t, lab, d, &ResendSummary{Report: NewID(), Disposition: DispositionPublished, Decision: d.ID}, false)
		},
		"renamed done": func(d Decision) string {
			return writeResend(t, lab, d, &ResendSummary{Report: NewID(), Disposition: DispositionDecided, Decision: d.ID, Child: NewID()}, true)
		},
	}
	var paths []string
	for _, form := range forms {
		d := lab.decision(t, ChoiceNotArrived)
		paths = append(paths, form(d))
	}
	if err := WriteJournal(lab.dir, "api", "other"+doneSuffix, TurnJournal{Epoch: lab.epoch, Done: true}); err != nil {
		t.Fatal(err)
	}
	swept := filepath.Join(JournalPath(lab.dir, "api"), "other"+doneSuffix)
	for _, live := range []string{lab.epoch, "next-epoch"} {
		for _, path := range append([]string{swept}, paths...) {
			if err := os.Chtimes(path, old, old); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
		sweepTurnRecords(lab.dir, "api", live, time.Now().Add(-keepFinished))
		for _, path := range paths {
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("a sweep with %s live removed the resend %s: %v", live, filepath.Base(path), err)
			}
		}
	}
	if _, err := os.Stat(swept); !os.IsNotExist(err) {
		t.Fatalf("an ended run's done journal was kept beside the resends: %v", err)
	}
}

// A decision disposes the occurrences it names about its own report and no
// other: not a second occurrence of the same report it did not name, not one
// of another report or of its journal as a whole, not one whose record or
// resolution does not read; and a decision that does not read disposes
// nothing.
func TestDecisionDisposesOnlyTheOccurrencesItNames(t *testing.T) {
	lab := newDecisionLab(t)
	w := live(lab.dir)
	op := "end/" + lab.report.ID
	record := func(cause stopCause) openStop {
		t.Helper()
		stop, err := w.recordOccurrence("api", cause)
		if err != nil {
			t.Fatal(err)
		}
		return stop
	}
	named := record(stopCause{Kind: causeEffect, Op: op, Paths: []string{"inbox/web/a"}, Cause: "named"})
	// A second occurrence of the named one's key: found again after the first.
	unnamed := record(stopCause{Kind: causeEffect, Op: op, Paths: []string{"inbox/web/a"}, Cause: "named"})
	journal := record(stopCause{Kind: causeEffect, Op: "end", Paths: []string{"inbox/web/c"}, Cause: "the journal"})
	other := record(stopCause{Kind: causeEffect, Op: "end/" + NewID(), Paths: []string{"inbox/web/d"}, Cause: "another report"})
	broken := record(stopCause{Kind: causeEffect, Op: op, Paths: []string{"inbox/web/e"}, Cause: "unreadable"})
	writeRaw(t, broken.path(lab.dir, "api"), `{`)
	unresolved := record(stopCause{Kind: causeEffect, Op: op, Paths: []string{"inbox/web/f"}, Cause: "resolution unreadable"})
	writeRaw(t, unresolved.path(lab.dir, "api")+resolvedSuffix, `{`)

	d := lab.decision(t, ChoiceArrived)
	d.Disposes = nil
	for _, stop := range []openStop{named, journal, other, broken, unresolved} {
		d.Disposes = append(d.Disposes, Disposal{Key: stop.key, Occurrence: stop.id})
	}
	path := writeDecision(t, lab.dir, d)
	if err := w.makeDurable("api", path, d); err != nil {
		t.Fatal(err)
	}

	// The disposed are what stopState, the one reader of a stop, no longer
	// answers open.
	disposedIDs := func() map[string]bool {
		t.Helper()
		open, err := w.stopState("api")
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		for _, stop := range []openStop{named, unnamed, journal, other, broken, unresolved} {
			if !slices.ContainsFunc(open, func(o openStop) bool { return o.id == stop.id }) {
				ids[stop.id] = true
			}
		}
		return ids
	}
	if got := disposedIDs(); len(got) != 1 || !got[named.id] {
		t.Fatalf("the decision disposed %v, not only %s (unnamed %s)", got, named.id, unnamed.id)
	}
	whole, _ := json.Marshal(d)
	for _, broken := range []string{strings.Replace(string(whole), deliveryUnproven, "proven", 1), `{`} {
		writeRaw(t, path, broken)
		if got := disposedIDs(); len(got) != 0 {
			t.Fatalf("a decision that does not read disposed %v", got)
		}
	}
}
