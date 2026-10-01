package cli

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// A command that changes something through the tool's words — a heads-up, a
// pending mark — runs under a receipt (docs/mail-bridge-cli.md#receipts):
// journaled before its effect, finished with its answer. The same words in the
// same turn find that receipt and get its answer or finish its steps; a tool
// call that timed out is finished by rewake retry <receipt>. The shell uses the
// same journal. It knows no native turn, so its calls are never joined by their
// words, and a shell call whose words match an operation still open does not
// run beside it: it names the receipt that finishes it.

// operation is a command running under its receipt.
type operation struct {
	site   readSite
	record receipt.Record
	// first says this attempt is the call that created the record, the one
	// attempt that runs in the operation's own turn whatever the transport
	// (docs/mail-bridge-turns.md#a-pending-mark-at-its-turns-end).
	first bool
}

// save writes the record's progress.
func (op *operation) save() error {
	if err := receipt.Save(op.site.dir, op.site.self.Name, op.record); err != nil {
		return failf("could not journal the step, so it was not taken: %v", err)
	}
	return nil
}

// unfinishedError is a failure that leaves its operation open: its effect was
// not committed, or not recorded as committed, and rewake retry finishes it.
type unfinishedError struct{ message string }

func (e *unfinishedError) Error() string { return e.message }

// journaled runs a handler under the receipt its call scopes. A process that
// is no session has nothing to journal and runs the handler as before, which
// refuses what a session alone may do.
func journaled(ctx *Context, call Call, handler func(*Context, Call) error) error {
	ctx.journaling = true
	dir, err := state.Dir()
	if err != nil {
		return handler(ctx, call)
	}
	self, epoch, err := ownRun(dir)
	if err != nil {
		return handler(ctx, call)
	}
	site := readSite{dir: dir, self: self, epoch: epoch}
	words := normalized(parsed{Call: call})
	digest := bridge.Digest(words)
	key := receipt.Key{Epoch: epoch, Digest: digest}
	fresh := receipt.Record{Words: words, Transport: receipt.Shell}
	if scope := ctx.scope; scope != nil {
		key.Conversation, key.Turn = scope.ticket.Conversation, scope.ticket.Turn
		fresh.Transport, fresh.CalledBoot = scope.ticket.Transport, scope.ticket.CalledBoot
	}
	// The same words in the same turn join their operation. Anything else —
	// the shell, or a tool call whose turn holds no operation of these words
	// — first stops at one of an earlier call that is not settled
	// (docs/mail-bridge-turns.md#the-turn-a-call-belongs-to).
	_, found, err := receipt.Joined(dir, self.Name, key)
	if err != nil {
		return failf("could not read this run's receipts (%v), so whether these words began an operation that is not finished is unknown; nothing was done", err)
	}
	if !found {
		if err := unresolvedBefore(site, digest); err != nil {
			return err
		}
	}
	record, joined, err := receipt.Begin(dir, self.Name, key, fresh)
	if err != nil {
		return failf("could not journal the call, so it was not made: %v", err)
	}
	return runOperation(ctx, call, handler, site, record.Token, !joined)
}

// unresolvedBefore stops a call whose words match an operation of this run
// whose effect is not settled and which it does not join: a shell call, or a
// tool call of a later turn. It may be the one a tool call lost with its
// answer, or one that is running now, or an earlier call's that stopped short,
// or one whose effect nobody can tell any more; neither caller can tell which.
// Doing it again beside that one is the outcome that cannot be taken back, so
// the call is accepted and not done, and names the receipt — whatever its age.
// A journal that cannot be read may hold such an operation, so it stops the
// call as well.
func unresolvedBefore(site readSite, digest string) error {
	found, err := receipt.Unresolved(site.dir, site.self.Name, site.epoch, digest)
	if err != nil {
		return failf("could not read this run's receipts (%v), so whether these words began an operation that is not finished is unknown; nothing was done", err)
	}
	if len(found) == 0 {
		return nil
	}
	var open, uncertain []string
	for _, record := range found {
		if record.Phase == receipt.Open {
			open = append(open, record.Token)
		} else {
			uncertain = append(uncertain, record.Token)
		}
	}
	if len(open) > 0 {
		return &PendingError{Message: fmt.Sprintf("Rewake: these same words began an operation of this run that is not finished (receipt %s), so they were not run again beside it; its effect may be done already. Finish it, or see its answer, with: rewake retry %s",
			strings.Join(open, ", "), open[0])}
	}
	return failf("these same words began an operation of this run whose effect is unknown (receipt %s), so they are not run again while this run lives; its answer: rewake retry %s. A new message in other words is a new operation.",
		strings.Join(uncertain, ", "), uncertain[0])
}

// runOperation holds the record and runs the handler once, or replays the
// answer a finished operation gave.
func runOperation(ctx *Context, call Call, handler func(*Context, Call) error, site readSite, token string, first bool) error {
	release, err := lockOperation(ctx, site, token)
	if err != nil {
		return err
	}
	defer release()
	record, err := receipt.Load(site.dir, site.self.Name, site.epoch, token)
	if err != nil {
		return failf("could not read the receipt %s: %v", token, err)
	}
	if ctx.scope != nil && !contains(record.Calls, ctx.scope.ticket.CallID) {
		record.Calls = append(record.Calls, ctx.scope.ticket.CallID)
	}
	op := &operation{site: site, record: record, first: first}
	if record.Phase == receipt.Done && record.Outcome != nil {
		return replay(ctx, op)
	}
	var out, errOut bytes.Buffer
	inner := &Context{Stdout: &out, Stderr: &errOut, JSON: ctx.JSON, scope: ctx.scope, op: op, journaling: true}
	failure := handler(inner, call)
	code := ExitOK
	if failure != nil {
		code = report(inner, failure)
	}
	var unfinished *unfinishedError
	var saveErr error
	if errors.As(failure, &unfinished) {
		saveErr = op.save()
	} else {
		// Done, or refused with its effect proven absent: either way the
		// answer is final, and the same words in this turn get it again.
		// A done answer whose record could not be written is still done:
		// the record stays open, and finishing it changes nothing again.
		op.record.Phase = receipt.Done
		op.record.Outcome = &receipt.Outcome{Exit: code, Stdout: out.String(), Stderr: errOut.String()}
		if err := receipt.Save(site.dir, site.self.Name, op.record); err != nil {
			saveErr = failf("done, but its receipt could not be written (%v); record it with: rewake retry %s", err, op.record.Token)
		}
	}
	_, _ = ctx.Stdout.Write(out.Bytes())
	_, _ = ctx.Stderr.Write(errOut.Bytes())
	if ctx.scope != nil && !ctx.JSON {
		_, _ = fmt.Fprintf(ctx.Stdout, "Rewake: receipt %s.\n", op.record.Token)
	}
	if saveErr != nil && code == ExitOK {
		_, _ = fmt.Fprintln(ctx.Stderr, saveErr.Error())
		return &ExitCodeError{Code: ExitFailed}
	}
	if code != ExitOK {
		return &ExitCodeError{Code: code}
	}
	return nil
}

// replay prints the answer a finished operation gave, which is what a repeat
// of its words gets: the effect happened once.
func replay(ctx *Context, op *operation) error {
	_ = op.save()
	if step := op.record.Pending; step != nil && step.Marked && !inOwnTurn(ctx, op) {
		// The mark speaks for its own turn only: told to a later one, it
		// would say that turn waits when it does not.
		return failf("these words marked a turn earlier (receipt %s), and this mark was for an earlier turn, or one this call cannot name; this turn is not marked by it", op.record.Token)
	}
	if !ctx.JSON {
		_, _ = fmt.Fprintf(ctx.Stdout, "Rewake: these words ran earlier (receipt %s); nothing was done again, and their answer follows.\n", op.record.Token)
	}
	outcome := op.record.Outcome
	_, _ = ctx.Stdout.Write([]byte(outcome.Stdout))
	_, _ = ctx.Stderr.Write([]byte(outcome.Stderr))
	if outcome.Exit != ExitOK {
		return &ExitCodeError{Code: outcome.Exit}
	}
	return nil
}

// beforeCommit refuses to take an effect once the call's answer is no longer
// wanted: a result that arrives after the harness gave up on it is one the
// model never sees, and the operation stays open for rewake retry <token>.
// It is called inside the critical section, just before the effect: a check
// made before a lock wait is stale once the lock is held.
func beforeCommit(ctx *Context, token, what string) error {
	if ctx.scope == nil || ctx.scope.remaining() > 0 {
		return nil
	}
	return &unfinishedError{message: fmt.Sprintf("Rewake: this tool call's deadline came before %s, so it was not done; do it with: rewake retry %s", what, token)}
}
