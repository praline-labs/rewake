package mail

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A decision authorizes nothing until it is durable: its file, the decisions
// directory and the mailbox's flushed. Whichever of those flushes keeps
// failing — the decisions directory's among them, after which the decision's
// link is visible and its publication answered an error — the barrier stops
// every time on that decision, whichever its choice: no resend is linked, no
// note is told, the occurrence it disposes stays open, no marker says it
// durable, the evidence of the installation does not take it for installed,
// and the original journal's tail does not run. Once the flush succeeds the
// barrier makes it durable and installs it once, and the occurrence is
// disposed.
func TestDecisionEffectsWaitForTheDecisionToBeDurable(t *testing.T) {
	for _, run := range []struct{ flushed, choice string }{
		{"the decision", ChoiceNotArrived},
		{"the decisions directory", ChoiceNotArrived},
		{"the mailbox", ChoiceNotArrived},
		{"the decision", ChoiceArrived},
		{"the decisions directory", ChoiceArrived},
		{"the mailbox", ChoiceArrived},
	} {
		flushed := run.flushed
		t.Run(flushed+", "+run.choice, func(t *testing.T) {
			lab := newDecisionLab(t)
			op := "end/" + lab.report.ID
			stop, err := live(lab.dir).recordOccurrence("api", stopCause{Kind: causeEffect, Op: op, Paths: []string{"inbox/web/a"}, Cause: "the report's outcome"})
			if err != nil {
				t.Fatal(err)
			}
			d := lab.decision(t, run.choice)
			d.Disposes = []Disposal{{Key: stop.key, Occurrence: stop.id}}
			path := DecisionPath(lab.dir, "api", lab.report.ID)
			if err := state.EnsureSubdir(filepath.Dir(path)); err != nil {
				t.Fatal(err)
			}
			at := map[string]string{
				"the decision":            path,
				"the decisions directory": filepath.Dir(path),
				"the mailbox":             state.InboxPath(lab.dir, "api"),
			}[flushed]
			failed := 0
			state.FlushFault = func(path string) error {
				if path == at {
					failed++
					return syscall.EIO
				}
				return nil
			}
			t.Cleanup(func() { state.FlushFault = nil })
			raw, _ := json.Marshal(d)
			err = state.PublishExclusive(path, raw)
			linkedUnflushed := flushed == "the decisions directory"
			if linkedUnflushed && !errors.Is(err, syscall.EIO) || !linkedUnflushed && err != nil {
				t.Fatalf("the decision's publication answered %v", err)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("the decision is not visible: %v", err)
			}
			resent, _ := d.resendJournal()
			for range 2 {
				if err := finishAll(t, lab.dir); err == nil {
					t.Fatal("the barrier went past a decision not made durable")
				}
			}
			if failed == 0 {
				t.Fatal("no flush was asked for")
			}
			if live(lab.dir).resendNamed("api", d.ID) || fileExists(path+durableSuffix) || fileExists(path+closedSuffix) ||
				fileExists(toldPath(lab.dir, "api", lab.report.ID, ToldRecipient)) {
				t.Fatal("an effect, a marker or a closure was written on a decision not made durable")
			}
			if letters := lettersOf(t, lab.dir, "web"); len(letters) != 0 {
				t.Fatalf("web holds %v", letters)
			}
			if fileExists(filepath.Join(JournalPath(lab.dir, "api"), "end"+doneSuffix)) || live(lab.dir).installed("api") {
				t.Fatal("the original journal ran, or the installation's evidence took it for installed")
			}
			if !slices.ContainsFunc(stopsOnRecord(t, lab.dir, "api"), func(o openStop) bool { return o.id == stop.id }) {
				t.Fatal("a decision not made durable disposed its occurrence")
			}

			state.FlushFault = nil
			for range 2 {
				err = finishAll(t, lab.dir)
			}
			if err != nil {
				t.Fatalf("past the flush the barrier answers %v", err)
			}
			if _, err := readDurable(osAccess{}, path+durableSuffix, d); err != nil {
				t.Fatalf("no marker: %v", err)
			}
			letters := lettersOf(t, lab.dir, "web")
			if run.choice == ChoiceNotArrived && letters[resent.Reports[0].ID] != 1 || letters[d.Notes[0].Note.ID] != 1 || letters[lab.report.ID] != 0 {
				t.Fatalf("web holds %v", letters)
			}
			if slices.ContainsFunc(stopsOnRecord(t, lab.dir, "api"), func(o openStop) bool { return o.id == stop.id }) {
				t.Fatal("the durable decision did not dispose its occurrence")
			}
		})
	}
}

// A marker is the decision's own: one that does not read or names another
// decision is an unknown — no effect, no disposal, never written over.
func TestDecisionMarkerOfAnotherDecisionMakesNothingDurable(t *testing.T) {
	for name, raw := range map[string]string{
		"does not read":    "{",
		"another decision": `{"decision":"` + NewID() + `","at":"2026-10-12T00:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			lab := newDecisionLab(t)
			d := lab.decision(t, ChoiceNotArrived)
			path := writeDecision(t, lab.dir, d)
			writeRaw(t, path+durableSuffix, raw)
			if err := finishAll(t, lab.dir); err == nil {
				t.Fatal("the barrier went past it")
			}
			if live(lab.dir).resendNamed("api", d.ID) || len(lettersOf(t, lab.dir, "web")) != 0 || live(lab.dir).installed("api") {
				t.Fatal("it made the decision's effects")
			}
			if after, _ := os.ReadFile(path + durableSuffix); string(after) != raw {
				t.Fatal("the marker was written over")
			}
		})
	}
}
