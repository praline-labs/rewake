package mail

import "fmt"

// What a decision that was asked for and not made, or made and not finished,
// comes to (docs/v2/stage3-decision.md#the-command). The kinds are what a
// caller answers by: a wrong call, a refusal with nothing written, or a
// record that exists while something after it did not go through.

// DecisionOutcome names why a decision was not made, or not finished.
type DecisionOutcome string

const (
	// DecisionNoSubject: the report is no subject — no open occurrence about
	// it, its journal gone or unreadable, its step settled, two journals
	// naming it. A wrong call.
	DecisionNoSubject DecisionOutcome = "no-subject"
	// DecisionUnauthorized: the caller was found positively unable to decide
	// this subject. A wrong call.
	DecisionUnauthorized DecisionOutcome = "unauthorized"
	// DecisionChanged: the subject under the lock is not the one the caller
	// found before it asked. Nothing written.
	DecisionChanged DecisionOutcome = "changed"
	// DecisionBusy: a mailbox lock was not had within its wait. Nothing
	// written; it is never a reason to decide.
	DecisionBusy DecisionOutcome = "busy"
	// DecisionOpposite: a decision with the other choice exists; it is never
	// replaced.
	DecisionOpposite DecisionOutcome = "opposite"
	// DecisionProven: the outcome was proven since the stop; the barrier
	// settles it by evidence.
	DecisionProven DecisionOutcome = "proven"
	// DecisionUnreadable: something the decision stands on could not be
	// read — a decision already there, the room's registry, the subject's
	// records — or its reads pass what a decision may keep. Nothing written.
	DecisionUnreadable DecisionOutcome = "unreadable"
	// DecisionUncertain: the record may exist, or exists and is not yet
	// durable; nothing it fixes is installed until a later writer makes it
	// durable. It is never removed or replaced.
	DecisionUncertain DecisionOutcome = "uncertain"
	// DecisionEffectFailed: the record is durable and an effect it fixes
	// could not be installed; the next barrier installs it.
	DecisionEffectFailed DecisionOutcome = "effect-failed"
)

// DecisionError is a decision not made, or made and not finished.
type DecisionError struct {
	Outcome DecisionOutcome
	// Recorded says the decision's record is known to be on disk.
	Recorded bool
	// Path is the record the outcome is about: the decision that may exist,
	// or the one that could not be read.
	Path string
	Err  error
}

func (e *DecisionError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%v (%s)", e.Err, e.Path)
	}
	return e.Err.Error()
}

func (e *DecisionError) Unwrap() error { return e.Err }

func refuseDecision(outcome DecisionOutcome, path string, format string, args ...any) *DecisionError {
	return &DecisionError{Outcome: outcome, Path: path, Err: fmt.Errorf(format, args...)}
}
