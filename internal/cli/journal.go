package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A command that changes something — a heads-up, a pending mark — runs under a
// receipt: journaled before its effect, finished with its answer, and an
// operation left open is finished by rewake retry <receipt>. The shell knows
// no native turn, so its calls are never joined by their words, and a call
// whose words match an operation still open does not run beside it: it names
// the receipt that finishes it.

// operation is a command running under its receipt.
type operation struct {
	site   readSite
	record receipt.Record
	// first says this attempt is the call that created the record, the one
	// attempt that runs in the operation's own turn.
	first bool
	// model is the answer's model as JSON, kept in the outcome.
	model string
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
	words := ctx.table.normalized(parsed{Call: call})
	digest := receipt.Digest(words)
	key := receipt.Key{Epoch: epoch, Digest: digest}
	fresh := receipt.Record{Words: words, Transport: receipt.Shell}
	// A call first stops at an operation of an earlier call with these words
	// that is not settled.
	if err := unresolvedBefore(site, digest); err != nil {
		return err
	}
	record, err := receipt.Begin(dir, self.Name, key, fresh)
	if err != nil {
		return failf("could not journal the call, so it was not made: %v", err)
	}
	return runOperation(ctx, call, handler, site, record.Token, true)
}

// unresolvedBefore stops a call whose words match an operation of this run
// whose effect is not settled. It may be one that
// is running now, or an earlier call's that stopped short, or one whose effect
// nobody can tell any more; the caller cannot tell which.
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
	release, err := lockOperation(site, token)
	if err != nil {
		return err
	}
	defer release()
	record, err := receipt.Load(site.dir, site.self.Name, site.epoch, token)
	if err != nil {
		return failf("could not read the receipt %s: %v", token, err)
	}
	op := &operation{site: site, record: record, first: first}
	if record.Phase == receipt.Done && record.Outcome != nil {
		return replay(ctx, op)
	}
	var out, errOut bytes.Buffer
	inner := &Context{Stdout: &out, Stderr: &errOut, JSON: ctx.JSON, op: op, journaling: true, table: ctx.table}
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
		// answer is final, and rewake retry gets it again.
		// A done answer whose record could not be written is still done:
		// the record stays open, and finishing it changes nothing again.
		op.record.Phase = receipt.Done
		op.record.Outcome = &receipt.Outcome{Exit: code, Stdout: out.String(), Stderr: errOut.String(), Model: op.model}
		if err := receipt.Save(site.dir, site.self.Name, op.record); err != nil {
			saveErr = failf("done, but its receipt could not be written (%v); record it with: rewake retry %s", err, op.record.Token)
		}
	}
	_, _ = ctx.Stdout.Write(out.Bytes())
	_, _ = ctx.Stderr.Write(errOut.Bytes())
	if saveErr != nil && code == ExitOK {
		_, _ = fmt.Fprintln(ctx.Stderr, saveErr.Error())
		return &ExitCodeError{Code: ExitFailed}
	}
	if code != ExitOK {
		return &ExitCodeError{Code: code}
	}
	return nil
}

// replay prints the answer a finished operation gave, which is what a retry
// of its receipt gets: the effect happened once.
func replay(ctx *Context, op *operation) error {
	_ = op.save()
	if step := op.record.Pending; step != nil && step.Marked && !inOwnTurn(op) {
		// The mark speaks for its own turn only: told to a later one, it
		// would say that turn waits when it does not.
		return failf("these words marked a turn earlier (receipt %s), and this mark was for an earlier turn, or one this call cannot name; this turn is not marked by it", op.record.Token)
	}
	if !ctx.JSON {
		_, _ = fmt.Fprintf(ctx.Stdout, "Rewake: these words ran earlier (receipt %s); nothing was done again, and their answer follows.\n", op.record.Token)
	}
	outcome := op.record.Outcome
	if ctx.JSON && outcome.Model != "" {
		_, _ = fmt.Fprintln(ctx.Stdout, outcome.Model)
	} else {
		_, _ = ctx.Stdout.Write([]byte(outcome.Stdout))
	}
	_, _ = ctx.Stderr.Write([]byte(outcome.Stderr))
	if outcome.Exit != ExitOK {
		return &ExitCodeError{Code: outcome.Exit}
	}
	return nil
}

// readSite is the run an operation belongs to.
type readSite struct {
	dir   string
	self  registry.Session
	epoch string
}

// lockOperation holds a record for as long as the call's answer is wanted.
// Every path that holds a record comes through here.
func lockOperation(site readSite, token string) (func(), error) {
	lockCtx, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	release, err := receipt.Lock(lockCtx, site.dir, site.self.Name, site.epoch, token)
	if errors.Is(err, receipt.ErrBusy) {
		return nil, &PendingError{Message: fmt.Sprintf("Rewake: the same operation is running in another call right now; its answer: rewake retry %s", token)}
	}
	if err != nil {
		return nil, failf("could not lock the receipt %s: %v", token, err)
	}
	return release, nil
}

// normalized is the canonical form of a call: the command, its flags sorted
// with their values inline, then its positionals after --. Two spellings of
// one call normalize alike; texts stay exactly as given.
func (t *table) normalized(result parsed) []string {
	call := result.Call
	words := []string{call.Command.Name}
	names := make([]string, 0, len(call.Flags))
	for name := range call.Flags {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if t.takesValue(call.Command, name) {
			words = append(words, "--"+name+"="+call.Flags[name])
		} else {
			words = append(words, "--"+name)
		}
	}
	if len(call.Positionals) > 0 {
		words = append(append(words, "--"), call.Positionals...)
	}
	return words
}
