package mail

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// withinBound runs fn and fails the test when it waits past a few seconds
// for something no reader may wait on: a FIFO's writer, under the mailbox
// lock. release frees what fn waits on, so the test joins it either way.
func withinBound(t *testing.T, fn func() error, release func()) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		release()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		t.Fatal("a reader waited on a FIFO while holding the mailbox lock")
		return nil
	}
}

// releaseFIFO opens the FIFO at path for writing, which frees a reader
// waiting on it, and replaces it with bytes that do not read.
func releaseFIFO(t *testing.T, path string) func() {
	return func() {
		fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			_ = os.Remove(path)
			_ = state.WriteAtomic(path, []byte("{"))
			_, _ = syscall.Write(fd, []byte("{"))
			_ = syscall.Close(fd)
		}
	}
}

// anotherSubject is a second report of the lab's sender, in a journal of its
// own, so a decision on it is independent of one on the lab's report.
func anotherSubject(t *testing.T, lab decisionLab) Message {
	t.Helper()
	report := lab.report
	report.ID, report.InReplyTo, report.Text = NewID(), nil, "another answer"
	if err := WriteJournal(lab.dir, "api", "end2", TurnJournal{Epoch: lab.epoch, Reports: []Message{report}}); err != nil {
		t.Fatal(err)
	}
	return report
}

// A decision is read as a regular file of its mailbox and nothing else, by
// the installation that runs before the reading of the mailbox: a link in its
// place is not followed and a FIFO is not waited on, a directory in its place
// or in the place of the decisions is an unknown, and none of them installs a
// resend or a note. A valid decision on another report installs beside it, as
// each installs on its own; the barrier stops, and the views name each
// without waiting.
func TestDecisionSpecialFilesAuthorizeNothing(t *testing.T) {
	for _, place := range []string{"a link to a decision", "a FIFO", "a directory"} {
		t.Run(place, func(t *testing.T) {
			lab := newDecisionLab(t)
			d := lab.decision(t, ChoiceNotArrived)
			other := anotherSubject(t, lab)
			valid := newDecision(t, lab.dir, "end2", lab.epoch, other, ChoiceNotArrived)
			writeDecision(t, lab.dir, valid)
			path := DecisionPath(lab.dir, "api", lab.report.ID)
			release := func() {}
			switch place {
			case "a link to a decision":
				raw, _ := json.Marshal(d)
				foreign := filepath.Join(lab.dir, "foreign-decision")
				if err := state.WriteAtomic(foreign, raw); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(foreign, path); err != nil {
					t.Fatal(err)
				}
			case "a FIFO":
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
				release = releaseFIFO(t, path)
			case "a directory":
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() { _ = os.RemoveAll(path) })
			if err := withinBound(t, func() error { return finishAll(t, lab.dir) }, release); err == nil {
				t.Error("the barrier went past it")
			}
			if live(lab.dir).resendNamed("api", d.ID) || lettersOf(t, lab.dir, "web")[d.Notes[0].Note.ID] != 0 {
				t.Error("it installed a resend or told a note")
			}
			if !live(lab.dir).resendNamed("api", valid.ID) || lettersOf(t, lab.dir, "web")[valid.Notes[0].Note.ID] != 1 {
				t.Error("the valid decision beside it installed nothing")
			}
			for _, id := range []string{"end", "end2"} {
				if fileExists(filepath.Join(JournalPath(lab.dir, "api"), id+doneSuffix)) {
					t.Errorf("the journal %s ran", id)
				}
			}
			var views []DecisionView
			err := withinBound(t, func() error { var err error; views, err = Decisions(lab.dir, "api"); return err }, release)
			if err != nil || len(views) != 2 {
				t.Fatalf("the views: %v %+v", err, views)
			}
			for _, view := range views {
				if (view.Report == lab.report.ID) != (view.Err != nil) {
					t.Errorf("the view of %s says %v", view.Report, view.Err)
				}
			}
		})
	}

	for _, place := range []string{"a link", "a file"} {
		t.Run("the decisions are "+place, func(t *testing.T) {
			lab := newDecisionLab(t)
			d := lab.decision(t, ChoiceNotArrived)
			raw, _ := json.Marshal(d)
			root := filepath.Dir(DecisionPath(lab.dir, "api", lab.report.ID))
			switch place {
			case "a link":
				elsewhere := filepath.Join(lab.dir, "elsewhere")
				if err := state.EnsureSubdir(elsewhere); err != nil {
					t.Fatal(err)
				}
				if err := state.WriteAtomic(filepath.Join(elsewhere, lab.report.ID), raw); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(elsewhere, root); err != nil {
					t.Fatal(err)
				}
			case "a file":
				writeRaw(t, root, string(raw))
			}
			t.Cleanup(func() { _ = os.Remove(root) })
			if err := finishAll(t, lab.dir); err == nil {
				t.Error("the barrier went past it")
			}
			if live(lab.dir).resendNamed("api", d.ID) || lettersOf(t, lab.dir, "web")[d.Notes[0].Note.ID] != 0 {
				t.Error("decisions not in their place installed a resend or told a note")
			}
			if _, err := Decisions(lab.dir, "api"); err == nil {
				t.Error("the view listed decisions not in their place")
			}
		})
	}
}

// The records beside a decision are read the same way: a told record or a
// closure that is a FIFO or a link is an unknown, read without waiting, never
// taken for told or closed and never written over.
func TestDecisionRecordsBesideItAreReadAsRegularFiles(t *testing.T) {
	for _, record := range []string{toldRecipientSuffix, closedSuffix} {
		for _, place := range []string{"a FIFO", "a link"} {
			t.Run(record+" as "+place, func(t *testing.T) {
				lab := newDecisionLab(t)
				d := lab.decision(t, ChoiceArrived)
				path := writeDecision(t, lab.dir, d) + record
				release := func() {}
				switch place {
				case "a FIFO":
					if err := syscall.Mkfifo(path, 0o600); err != nil {
						t.Fatal(err)
					}
					release = releaseFIFO(t, path)
				case "a link":
					if err := os.Symlink(filepath.Join(lab.dir, "nowhere"), path); err != nil {
						t.Fatal(err)
					}
				}
				t.Cleanup(func() { _ = os.Remove(path) })
				if err := withinBound(t, func() error { return finishAll(t, lab.dir) }, release); err == nil {
					t.Error("the barrier went past it")
				}
				if fileExists(filepath.Join(JournalPath(lab.dir, "api"), "end"+doneSuffix)) {
					t.Error("the journal ran")
				}
				if info, err := os.Lstat(path); err != nil || info.Mode().IsRegular() {
					t.Errorf("it was written over: %v", err)
				}
				if record == toldRecipientSuffix && lettersOf(t, lab.dir, "web")[d.Notes[0].Note.ID] != 0 {
					t.Error("the note was told beside a told record that does not read")
				}
			})
		}
	}
}

// swapAfterListing replaces a decision with a FIFO right after the decisions
// are listed, so the read that follows meets a FIFO the listing called a
// file.
type swapAfterListing struct {
	fileAccess
	root, path string
	swapped    bool
}

func (s *swapAfterListing) ReadDir(path string) ([]fs.DirEntry, error) {
	entries, err := s.fileAccess.ReadDir(path)
	if path == s.root && !s.swapped {
		s.swapped = true
		_ = os.Remove(s.path)
		_ = syscall.Mkfifo(s.path, 0o600)
	}
	return entries, err
}

// A decision swapped for a FIFO between its listing and its read is read as
// what is there now: refused at once, nothing installed.
func TestDecisionSwappedAfterItsListingIsReadAsItIsNow(t *testing.T) {
	lab := newDecisionLab(t)
	d := lab.decision(t, ChoiceNotArrived)
	path := writeDecision(t, lab.dir, d)
	swap := &swapAfterListing{fileAccess: osAccess{}, root: filepath.Dir(path), path: path}
	testAccess.Store(lab.dir, passAccess{live: swap, plan: osAccess{}})
	t.Cleanup(func() { testAccess.Delete(lab.dir); _ = os.Remove(path) })
	if err := withinBound(t, func() error { return finishAll(t, lab.dir) }, releaseFIFO(t, path)); err == nil {
		t.Error("the barrier went past a decision swapped for a FIFO")
	}
	if !swap.swapped || live(lab.dir).resendNamed("api", d.ID) || lettersOf(t, lab.dir, "web")[d.Notes[0].Note.ID] != 0 {
		t.Errorf("swapped %v, and the swapped decision installed its effects", swap.swapped)
	}
}
