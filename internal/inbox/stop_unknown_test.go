package inbox

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

// A resolution lifts its occurrence only once it reads as one: bytes that do
// not parse, a valid resolution closed to reading, or one naming no evidence
// — no entry, or an entry empty, null or blank — keep the stop, and the barrier resolves nothing over it. One closed to
// reading lifts the stop once it reads again; the others hold it.
func TestAResolutionIsReadBeforeItLiftsTheStop(t *testing.T) {
	valid := `{"evidence":["inbox/api/pending/interim.json: read"],"at":"2026-10-05T00:00:00Z"}`
	for _, c := range []struct {
		name, contents string
		closed         bool
	}{
		{"invalid bytes", "{", false},
		{"no evidence", `{"evidence":[],"at":"2026-10-05T00:00:00Z"}`, false},
		{"empty evidence", `{"evidence":[""],"at":"2026-10-05T00:00:00Z"}`, false},
		{"null evidence", `{"evidence":[null],"at":"2026-10-05T00:00:00Z"}`, false},
		{"blank evidence", `{"evidence":["   "],"at":"2026-10-05T00:00:00Z"}`, false},
		{"one blank beside evidence", `{"evidence":["inbox/api/pending/interim.json: read","\t"],"at":"2026-10-05T00:00:00Z"}`, false},
		{"valid, closed to reading", valid, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newTwoSessionLab(t)
			path := validInterim(t, lab)
			if MailboxStopped(lab.dir, "api") == nil {
				t.Fatal("an unreadable interim did not stop the mailbox")
			}
			stops := stopsOnRecord(t, lab.dir, "api")
			if len(stops) != 1 {
				t.Fatalf("stops: %+v", stops)
			}
			readable(t, path)
			resolution := stops[0].path(lab.dir, "api") + resolvedSuffix
			writeRaw(t, resolution, c.contents)
			if c.closed {
				closeToReading(t, resolution)
			}
			for range 2 {
				err := MailboxStopped(lab.dir, "api")
				if err == nil || !strings.Contains(err.Error(), "whether it was resolved is unknown") {
					t.Fatalf("a resolution that does not read lifted the stop: %v", err)
				}
				if err := lab.reconcile(t); err == nil {
					t.Fatal("the barrier ran past a resolution that does not read")
				}
			}
			if c.closed {
				readable(t, resolution)
				if err := MailboxStopped(lab.dir, "api"); err != nil {
					t.Fatalf("the resolution reads again and the stop holds: %v", err)
				}
			}
		})
	}
}

// What an occurrence records of each path it names has three outcomes, as
// every check does (E6): there, not there, and unknown. Only a path found not
// there resolves as absent; one whose look failed is gone since with its
// presence unknown, which is no evidence.
func TestAnOccurrenceRecordsAPathsPresenceUnknownApartFromAbsent(t *testing.T) {
	dir := stateDir(t)
	// Outside every mailbox, whose readers would take these for strays.
	base := filepath.Join(dir, "probe")
	closed := filepath.Join(base, "closed")
	if err := os.MkdirAll(closed, 0o700); err != nil {
		t.Fatal(err)
	}
	there := filepath.Join(base, "there")
	writeRaw(t, there, "{}")
	paths := map[string]string{"there": there, "absent": filepath.Join(base, "absent"), "unknown": filepath.Join(closed, "unknown")}
	rel := map[string]string{}
	for what, path := range paths {
		rel[what] = relative(dir, path)
	}
	if err := os.Chmod(closed, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(closed, 0o700) })
	stop, err := live(dir).recordOccurrence("api", stopCause{Kind: causeUnreadable, Paths: []string{rel["there"], rel["absent"], rel["unknown"]}, Cause: "three paths"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(closed, 0o700); err != nil {
		t.Fatal(err)
	}
	record := stop.record
	if !slices.Equal(record.Present, []string{rel["there"]}) || !slices.Equal(record.Absent, []string{rel["absent"]}) {
		t.Fatalf("present %v, absent %v: want %s there, %s absent and %s in neither", record.Present, record.Absent, rel["there"], rel["absent"], rel["unknown"])
	}
	if err := os.Remove(there); err != nil {
		t.Fatal(err)
	}
	_, removed, err := live(dir).evidenceOf(stop)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(removed, []string{rel["there"], rel["unknown"]}) {
		t.Fatalf("removed %v: want the path gone since and the one whose presence was unknown", removed)
	}
	stop.record.Paths = []string{rel["absent"]}
	evidence, removed, err := live(dir).evidenceOf(stop)
	if err != nil || len(removed) != 0 || len(evidence) != 1 || !strings.Contains(evidence[0], "absent, as it was") {
		t.Fatalf("a path found absent then and now: evidence %v, removed %v, %v", evidence, removed, err)
	}
}

// "Not a directory" on the way to a path proves it absent at the look, as
// "no such file" does: a component is a file, so nothing stands below it.
// Once the obstruction is gone the path, still not there, is absent as it
// was. That takes nothing from a later reading: a path that was there, with
// a file now on its way, reads nothing yet, neither evidence nor removed.
func TestANotADirectoryOnTheWayRecordsThePathAbsent(t *testing.T) {
	dir := stateDir(t)
	base := filepath.Join(dir, "probe")
	obstruction := filepath.Join(base, "file")
	writeRaw(t, obstruction, "a file where a directory goes")
	child := relative(dir, filepath.Join(obstruction, "child"))
	stop, err := live(dir).recordOccurrence("api", stopCause{Kind: causeUnreadable, Paths: []string{child}, Cause: "a file on the way"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(stop.record.Absent, []string{child}) || len(stop.record.Present) != 0 {
		t.Fatalf("present %v, absent %v: want %s absent", stop.record.Present, stop.record.Absent, child)
	}
	if err := os.Remove(obstruction); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(obstruction, 0o700); err != nil {
		t.Fatal(err)
	}
	evidence, removed, err := live(dir).evidenceOf(stop)
	if err != nil || len(removed) != 0 || len(evidence) != 1 || !strings.Contains(evidence[0], "absent, as it was") {
		t.Fatalf("the child, still not there: evidence %v, removed %v, %v", evidence, removed, err)
	}

	there := filepath.Join(base, "dir", "there")
	writeRaw(t, there, "{}")
	stop, err = live(dir).recordOccurrence("api", stopCause{Kind: causeUnreadable, Paths: []string{relative(dir, there)}, Cause: "a path that was there"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Dir(there)); err != nil {
		t.Fatal(err)
	}
	writeRaw(t, filepath.Dir(there), "a file where its directory was")
	evidence, removed, err = live(dir).evidenceOf(stop)
	if err == nil || len(evidence) != 0 || len(removed) != 0 {
		t.Fatalf("a path that was there resolved past a file on its way: evidence %v, removed %v, %v", evidence, removed, err)
	}
}

// A report that landed and was read leaves its published mark as the only
// proof. A look that could not reach the mark records its presence unknown;
// the mark removed afterwards resolves nothing, and the barrier does not
// write the landed report again.
func TestAProofWhosePresenceWasUnknownResolvesNothingOnceGone(t *testing.T) {
	lab := newTwoSessionLab(t)
	lab.owe(t, "t1")
	report := lab.report(Finished, "t1")
	if _, err := PublishOnce(t.Context(), lab.dir, report, nil); err != nil {
		t.Fatal(err)
	}
	mark, _ := oncePath(lab.dir, report.To, report.ToEpoch, report.ID)
	if err := os.Remove(filepath.Join(state.InboxPath(lab.dir, report.To), report.ID+".json")); err != nil {
		t.Fatal(err)
	}
	lab.endTurn(t, report, "t1")
	parent := filepath.Dir(mark)
	if err := os.Chmod(parent, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	if MailboxStopped(lab.dir, "api") == nil {
		t.Fatal("a mark that cannot be reached did not stop the mailbox")
	}
	stops := stopsOnRecord(t, lab.dir, "api")
	if len(stops) != 1 || len(stops[0].record.Present) != 0 || len(stops[0].record.Absent) != 0 {
		t.Fatalf("the mark's presence is recorded as known: %+v", stops)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(mark); err != nil {
		t.Fatal(err)
	}
	err := lab.reconcile(t)
	if err == nil || !strings.Contains(err.Error(), "whether it was there when the stop was recorded is unknown") {
		t.Fatalf("the gone proof resolved its stop: %v", err)
	}
	if n := lab.copies(t, report.ID); n != 0 {
		t.Fatalf("the landed report was written again: %d copies", n)
	}
}

// Releasing an answer's reservation leaves its mark when an open occurrence
// names it: the mark is what the occurrence waits to read again. Once it
// reads, the stop resolves; with no stop, the release takes the mark.
func TestReleasingAnAnswerKeepsAMarkTheStopNames(t *testing.T) {
	for _, stopped := range []bool{true, false} {
		t.Run(map[bool]string{true: "stopped", false: "not stopped"}[stopped], func(t *testing.T) {
			lab := newTwoSessionLab(t)
			id := NewID()
			release, err := ReserveAnswer(lab.dir, "api", id)
			if err != nil {
				t.Fatal(err)
			}
			mark := filepath.Join(state.AnsweringPath(lab.dir, "api"), id)
			if stopped {
				closeToReading(t, mark)
				if MailboxStopped(lab.dir, "api") == nil {
					release()
					t.Fatal("an unreadable answering mark did not stop the mailbox")
				}
			}
			release()
			_, err = os.Stat(mark)
			if stopped != (err == nil) {
				t.Fatalf("stopped %v: the mark after release: %v", stopped, err)
			}
			if stopped {
				readable(t, mark)
				if err := MailboxStopped(lab.dir, "api"); err != nil {
					t.Fatalf("the mark reads again and the stop holds: %v", err)
				}
			}
		})
	}
}

// The server settles no letter an open occurrence names, by its own path or
// by a directory it lies in: neither the waiting copy of a delivered one nor
// either copy of a refused one moves. A letter the stop does not name
// settles as ever, and the named one settles once the stop is resolved.
func TestSettlingKeepsALetterTheStopNames(t *testing.T) {
	for _, named := range []string{"the letter", "its directory"} {
		for _, outcome := range []State{Delivered, Failed} {
			t.Run(named+", "+string(outcome), func(t *testing.T) {
				dir := stateDir(t)
				letters := []Message{{ID: NewID(), From: "web", To: "api", Kind: Task, Text: "named"}, {ID: NewID(), From: "web", To: "api", Kind: Task, Text: "other"}}
				for _, letter := range letters {
					if err := Put(dir, letter); err != nil {
						t.Fatal(err)
					}
					if err := linkUnread(dir, "api", letter.ID); err != nil {
						t.Fatal(err)
					}
				}
				waiting := func(id string) string { return filepath.Join(state.InboxPath(dir, "api"), id+".json") }
				path := waiting(letters[0].ID)
				if named == "its directory" {
					path = state.UnreadPath(dir, "api")
				}
				if outcome == Delivered && named == "its directory" {
					path = state.InboxPath(dir, "api")
				}
				stop, err := live(dir).recordOccurrence("api", stopCause{Kind: causeUnreadable, Paths: []string{relative(dir, path)}, Cause: named})
				if err != nil {
					t.Fatal(err)
				}
				settles := func(id string) bool {
					settle(dir, "api", id, outcome)
					_, err := os.Stat(waiting(id))
					return errors.Is(err, os.ErrNotExist)
				}
				if settles(letters[0].ID) {
					t.Fatal("a letter the stop names was settled")
				}
				if named == "the letter" && !settles(letters[1].ID) {
					t.Fatal("a letter the stop does not name stayed")
				}
				if err := live(dir).resolveStop("api", stop, []string{"read"}); err != nil {
					t.Fatal(err)
				}
				if !settles(letters[0].ID) {
					t.Fatal("the letter stayed after its stop was resolved")
				}
			})
		}
	}
}

// An effect's occurrence is resolved by the barrier running its operation
// through; one whose resolution does not read may be resolved or not, so the
// barrier holds its effects beside it rather than run them again. Without the
// resolution the barrier runs the journal through, which is what makes the
// first half mean anything.
func TestAnEffectsOccurrenceWhoseResolutionDoesNotReadHoldsTheEffects(t *testing.T) {
	for _, unreadable := range []bool{true, false} {
		t.Run(map[bool]string{true: "resolution unreadable", false: "no resolution"}[unreadable], func(t *testing.T) {
			lab := newTwoSessionLab(t)
			lab.owe(t, "t1")
			report := lab.report(Finished, "t1")
			lab.endTurn(t, report, "t1")
			stop, err := live(lab.dir).recordOccurrence("api", stopCause{Kind: causeEffect, Op: "end", Cause: "an effect met an unknown"})
			if err != nil {
				t.Fatal(err)
			}
			if unreadable {
				writeRaw(t, stop.path(lab.dir, "api")+resolvedSuffix, "{")
			}
			err = lab.reconcile(t)
			_, unfinished := os.Stat(filepath.Join(JournalPath(lab.dir, "api"), "end"))
			if unreadable && (err == nil || unfinished != nil || lab.copies(t, report.ID) != 0) {
				t.Fatalf("the barrier ran its effects beside a resolution that does not read: %v, journal %v, %d copies", err, unfinished, lab.copies(t, report.ID))
			}
			if !unreadable && (err != nil || unfinished == nil || lab.copies(t, report.ID) != 1) {
				t.Fatalf("the barrier did not run the journal through: %v, journal %v, %d copies", err, unfinished, lab.copies(t, report.ID))
			}
		})
	}
}
