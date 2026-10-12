package mail

import (
	"context"
)

// What follows a decision's record, made now or found made: its
// installation and the normal barrier, once, under the sender's lock (Q13 of
// docs/v2/stage3-decision-build.md). Reconcile is the owner of both — every
// barrier installs what the mailbox's decisions fixed before anything else —
// so a decision is not finished by a second machine of its own.

// decidedBefore answers a decision asked for again: the same choice makes the
// decision found durable, installs it and runs the barrier, the opposite is
// refused and nothing replaces the record. Who asks is held to the found
// decision's subject again — a retry that would write is authorized like the
// first ask — and so is the subject the caller found.
func decidedBefore(ctx context.Context, dir string, req DecideRequest, path string, d Decision) (DecideAnswer, error) {
	if d.Subject != req.Expect {
		return DecideAnswer{}, refuseDecision(DecisionChanged, path, "the decision on report %s is about %+v, not the subject found before, %+v", req.Report, d.Subject, req.Expect)
	}
	if err := authorize(req.By, d.Subject); err != nil {
		return DecideAnswer{}, err
	}
	if d.Choice != req.Choice {
		return DecideAnswer{}, refuseDecision(DecisionOpposite, path, "report %s was decided before as %s by decision %s, which nothing replaces", req.Report, d.Choice, d.ID)
	}
	return installDecided(ctx, dir, path, d, true)
}

// installDecided runs the barrier after d's record and says what came of it.
// A decision the barrier could not make durable is uncertain: it stands and
// fixes nothing yet. One durable whose resend is not installed is an effect
// that failed, recorded. Anything else the barrier answered — another cause
// the mailbox stays stopped on, a publication that waits — is the barrier's,
// and fails nothing the decision fixed.
func installDecided(ctx context.Context, dir, path string, d Decision, before bool) (DecideAnswer, error) {
	err := Reconcile(ctx, dir, d.Subject.Sender)
	w := live(dir)
	answer := DecideAnswer{Decision: d, Path: path, Before: before, Owed: w.owedNotes(d)}
	if err == nil {
		return answer, nil
	}
	if !w.durable(path, d) {
		return DecideAnswer{}, &DecisionError{Outcome: DecisionUncertain, Recorded: true, Path: path, Err: err}
	}
	if d.Choice == ChoiceNotArrived {
		if _, _, found, readErr := w.readResend(d.Subject.Sender, d); !found || readErr != nil {
			return DecideAnswer{}, &DecisionError{Outcome: DecisionEffectFailed, Recorded: true, Path: path, Err: err}
		}
	}
	answer.Barrier = err
	return answer, nil
}

// owedNotes are d's notes with no told record yet, by the run they tell.
func (w world) owedNotes(d Decision) []string {
	var owed []string
	for _, note := range d.Notes {
		_, err := readTold(w.files, toldPath(w.dir, d.Subject.Sender, d.Subject.Report, note.Told), note)
		if err != nil {
			owed = append(owed, note.Told)
		}
	}
	return owed
}
