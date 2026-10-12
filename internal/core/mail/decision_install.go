package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The barrier installs what each valid decision of its mailbox fixed before it
// looks at the stop or plans (Q7 of docs/v2/stage3-decision-build.md): the
// resend, linked whole from the decision's bytes at the name the decision
// fixed, and the notes, each told once. None of this waits for admission or
// for another decision, and none of it publishes a report of the stopped
// mailbox or clears a wait: recovery writes its own records and tells, which
// a stopped mailbox allows. Each effect is found installed by its identity —
// the resend's name in either form, a note's told record — never by a mark of
// progress, so a crash anywhere leaves each installed or not.

// installOp is the operation an unknown the installation met is about. No
// journal is named so — a name with a dot first is a write that never
// finished — and the barrier resolves such an occurrence once it has run the
// installation through (ranThrough).
const installOp = ".install"

// installDecisions installs the effects of every valid decision of name's
// mailbox not installed yet, each on its own. What it cannot read — the
// decisions, a decision, a told record, a resend's name — and a resend that
// could not be installed are errors, named, and no journal runs in this pass;
// a decision that does not read installs nothing, and the plan meets it as a
// cause of its own. A note that could not be told is no error: it stays owed
// and stops nothing, since a note answers no wait.
func (w world) installDecisions(ctx context.Context, name string) error {
	root, entries, err := w.decisionsOf(name)
	if err != nil {
		return inOp(installOp, fmt.Errorf("the decisions %s cannot be listed, so what they install is unknown: %w", root, err))
	}
	var failed []error
	for _, entry := range entries {
		if !isDecisionRecord(entry.Name()) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		d, err := readDecision(w.files, path)
		if err != nil {
			failed = append(failed, inOp(installOp, unknownRead(path, err)))
			continue
		}
		if err := w.makeDurable(name, path, d); err != nil {
			failed = append(failed, inOp(installOp, err))
			continue
		}
		if d.Choice == ChoiceNotArrived {
			if err := w.installResend(name, d); err != nil {
				failed = append(failed, inOp(installOp, err))
			}
		}
		for _, note := range d.Notes {
			if err := w.tellNote(ctx, name, d, note); err != nil {
				failed = append(failed, inOp(installOp, err))
			}
		}
	}
	return errors.Join(failed...)
}

// installResend links the resend a decision holds at journal/decided-<id>,
// unless that name is there in either form: the name is the installation, and
// nothing is written over it. What is there is held to the decision — bytes
// of another kind or another resend under its name are an unknown, never
// taken for installed — and a look that fails with anything but "no such
// file" is an unknown the effect met.
func (w world) installResend(name string, d Decision) error {
	journal := ResendJournal(d.ID)
	path := filepath.Join(JournalPath(w.dir, name), journal)
	if _, _, found, err := w.readResend(name, d); found || err != nil {
		return err
	}
	if err := w.ensureDir(JournalPath(w.dir, name)); err != nil {
		return fmt.Errorf("the resend of decision %s could not be installed: %w", d.ID, err)
	}
	err := w.publish(path, d.Resend)
	if err != nil && !errors.Is(err, state.ErrNameTaken) {
		return fmt.Errorf("the resend %s of decision %s could not be installed: %w", journal, d.ID, err)
	}
	return nil
}

// tellNote publishes a decision's note once and records it told, unless its
// told record is there. Only an answer that tells — written now, written
// before, the run proven ended — is recorded; any other leaves the note owed
// to a later barrier, and so does a told record that could not be written.
// A told record that does not read, or tells another note, name or run, is
// an unknown, never taken for told nor written over.
func (w world) tellNote(ctx context.Context, name string, d Decision, entry DecisionNote) error {
	path := toldPath(w.dir, name, d.Subject.Report, entry.Told)
	_, err := readTold(w.files, path, entry)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return unknownRead(path, err)
	}
	answer, err := w.publishNote(ctx, name, entry.Note)
	if err != nil || answer == "" {
		return nil
	}
	note := entry.Note
	raw, err := json.Marshal(ToldRecord{Name: note.To, Run: note.ToEpoch, Note: note.ID, Answer: answer, At: time.Now()})
	if err != nil {
		return nil
	}
	_ = w.publish(path, raw)
	return nil
}

// publishNote publishes a note as a report is published — inside its
// recipient's lock, under the once marks, through the held lock when it is to
// name itself — and answers what came of it. Its run is read there too, more
// strictly than a report's (noteRun): "" is a run not known to be running or
// ended, which leaves the note owed rather than tell it run-ended for good.
func (w world) publishNote(ctx context.Context, name string, note Message) (string, error) {
	put := world.putOnce
	if note.To == name {
		put = world.putLocal
	}
	answer := ""
	err := w.recipientSection(ctx, name, note.To, func() error {
		switch noteRun(w.dir, note) {
		case proc.IdentityEnded:
			answer = ToldEnded
			return nil
		case proc.IdentityUnknown:
			return nil
		}
		w.publicationStep(PublicationAdmitted, note)
		answer = ToldBefore
		return w.publishMarked(note, func(w world, m Message) error {
			answer = ToldWritten
			return put(w, m)
		})
	})
	if err != nil {
		return "", err
	}
	return answer, nil
}

// noteRun is what the run a note is to is found to be, inside its section: a
// note is told run-ended only on proof (Q11). The record of that run there
// and its session alive is running; its process proven ended — whatever holds
// the name now, or none — is ended; anything else, a record that does not
// read or a liveness that cannot be told among them, is unknown.
func noteRun(dir string, note Message) proc.IdentityEvidence {
	session, err := registry.Load(dir, note.To)
	if err == nil && session.Epoch() == note.ToEpoch && session.Alive() {
		return proc.IdentityAlive
	}
	if (err == nil || errors.Is(err, registry.ErrNotFound)) && registry.ObserveRun(note.ToEpoch) == proc.IdentityEnded {
		return proc.IdentityEnded
	}
	return proc.IdentityUnknown
}

// installed says the installation has nothing left that it could fail on:
// every decision reads and was made durable, every resend is there in either form as it holds, and
// every told record reads or is not there yet — a note owed is no failure.
// It is the evidence that resolves an occurrence about the installation, and
// the plan reads through it what the installation reads.
func (w world) installed(name string) bool {
	root, entries, err := w.decisionsOf(name)
	if err != nil {
		return false
	}
	done := true
	for _, entry := range entries {
		if !isDecisionRecord(entry.Name()) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		d, err := readDecision(w.files, path)
		if err != nil || !w.durable(path, d) {
			done = false
			continue
		}
		if d.Choice == ChoiceNotArrived {
			if _, _, found, err := w.readResend(name, d); !found || err != nil {
				done = false
			}
		}
		for _, note := range d.Notes {
			if _, err := readTold(w.files, toldPath(w.dir, name, d.Subject.Report, note.Told), note); err != nil && !errors.Is(err, os.ErrNotExist) {
				done = false
			}
		}
	}
	return done
}
