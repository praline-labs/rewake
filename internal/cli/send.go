package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/sessionstate"
	"github.com/praline-labs/rewake/internal/state"
)

// defaultWait is how long send waits for a result before reporting what it
// knows. It is short because the answer for a healthy delivery arrives in well
// under a second — a heads-up in a few, once it has waited for company
// (inbox.Coalescing, whose cap is set to fit inside this) — and a caller that
// has to wait longer wants to hear why.
const defaultWait = 5 * time.Second

// sendModel is the machine form of one send.
type sendModel struct {
	GrantGit bool `json:"grantGit,omitempty"`
	// GrantDirs are the directories the task grants, as resolved; GrantApplied
	// those the recipient's harness took with the notice, and
	// AlreadyWritable those found in the recipient's workspace, not carried.
	GrantDirs       []string               `json:"grantDirs,omitempty"`
	GrantApplied    []string               `json:"grantApplied,omitempty"`
	AlreadyWritable []string               `json:"alreadyWritable,omitempty"`
	Telemetry       *sessionstate.Snapshot `json:"telemetry,omitempty"`
	ID              string                 `json:"id"`
	To              string                 `json:"to"`
	From            string                 `json:"from"`
	State           string                 `json:"state"`
	Via             string                 `json:"via,omitempty"`
	Detail          string                 `json:"detail,omitempty"`
	// Answer is the receiver's last reply, for a question that got one.
	Answer        string     `json:"answer,omitempty"`
	Kind          inbox.Kind `json:"kind,omitempty"`
	ThreadChanged bool       `json:"threadChanged,omitempty"`
	// Addenda are the addenda that now add to an edit's replacement.
	Addenda []string `json:"addenda,omitempty"`
	// Replaces is the letter an edit replaced, and Named the id the edit was
	// given when that letter is the replacement of an earlier edit.
	Replaces string `json:"replaces,omitempty"`
	Named    string `json:"named,omitempty"`
	// Receipt names the journal record of a heads-up, for rewake retry.
	Receipt string `json:"receipt,omitempty"`
}

func handleSend(ctx *Context, call Call) error {
	command := call.Command
	if len(call.Positionals) < 2 {
		return &UsageError{
			Command: command,
			Message: "send needs a session name and the text to deliver.",
		}
	}
	target, text := call.Positionals[0], call.Positionals[1]

	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: command, Message: err.Error()}
	}
	// The flags first: a call that is wrong whoever it names is refused for
	// what is wrong with it, not for a session that happens to be missing.
	kind, err := chosenKind(call)
	if err != nil {
		return err
	}
	wait, err := waitDuration(call, kind.wait)
	if err != nil {
		return err
	}
	if kind.kind == inbox.Note && !ctx.journaling {
		// A heads-up is one of the tool's words: its receipt makes a repeat
		// or a retry publish it once.
		return journaled(ctx, call, handleSend)
	}
	if ctx.scope != nil {
		wait = max(0, min(wait, ctx.scope.remaining()-toolMargin))
	}
	// A resumed heads-up is the one its first call fixed: the same run, the
	// same text, stdin not read again.
	pinned := false
	if ctx.op != nil {
		if err := ctx.op.resolveEndedRun(ctx); err != nil {
			return err
		}
		if step := ctx.op.pinnedNotify(); step != nil {
			text, pinned = step.Text, true
		}
	}

	session, err := registry.Lookup(dir, target)
	if errors.Is(err, registry.ErrNotFound) {
		return unknownSessionError(dir, target)
	}
	if err != nil {
		return &FailedError{Message: err.Error()}
	}

	if text == "-" && !pinned {
		read, err := io.ReadAll(os.Stdin)
		if err != nil {
			return failf("could not read the message from stdin: %v", err)
		}
		text = string(read)
	}
	if strings.TrimSpace(text) == "" {
		return &UsageError{Command: command, Message: "The message is empty."}
	}

	if kind.kind == inbox.Question && role.Of(session.Role).Silent {
		return &UsageError{Command: command, Message: fmt.Sprintf("Session %s does not report its turns, so it cannot answer --question; use plain send or --notify instead.", session.Name)}
	}
	started := time.Now()

	// Signed with this session's name and run only when both are known to be
	// current: a report then reaches this run, and a process left over from an
	// earlier run cannot speak for the next one.
	self, epoch, selfErr := ownRun(dir)
	if selfErr == nil && self.Name == session.Name {
		// A task to itself would be announced into the turn that sent it and
		// owe a report to that same turn; a question would wait for an answer
		// only its own blocked turn could give.
		return &UsageError{Command: command, Message: fmt.Sprintf("a session cannot send to itself, and %s is this session: do the work in this turn, or send it to another session from rewake list.", session.Name)}
	}
	grantGit, grantErr := requestedGitGrant(call, self, session, selfErr)
	if grantErr != nil {
		return grantErr
	}
	grantDirs, grantErr := requestedDirGrants(call, self, session, selfErr, dir)
	if grantErr != nil {
		return grantErr
	}
	if kind.needsSession != "" && selfErr != nil {
		return &UsageError{
			Command: command,
			Message: kind.needsSession + " (" + selfErr.Error() + ").",
		}
	}

	// Reading stdin can take a while, and a session can end in that time. The
	// epoch pins the message to this run of the name, so a later session that
	// takes the same name does not receive somebody else's mail.
	if _, err := registry.Lookup(dir, session.Name); errors.Is(err, registry.ErrNotFound) {
		return unknownSessionError(dir, target)
	}

	addendumTo, err := addendumRoot(call, dir, kind, session)
	if err != nil {
		return err
	}

	message := inbox.Message{
		AddendumTo: addendumTo,
		GrantGit:   grantGit,
		GrantDirs:  grantDirs.dirs,
		GrantBroad: grantDirs.broad,
		ID:         inbox.NewID(),
		From:       harness.ShellSender,
		To:         session.Name,
		ToEpoch:    session.Epoch(),
		Kind:       kind.kind,
		Text:       text,
		CreatedAt:  time.Now(),
	}
	// Writing to a session does not settle what this one owes it. A message
	// sent mid-turn — "started", or a new task in answer to "ready" — is not the
	// end of the turn, and taking it for one lost the report the other side was
	// waiting for.
	if selfErr == nil {
		message.From, message.FromEpoch = self.Name, epoch
	}
	journaledSend := ctx.op != nil && message.AddendumTo == ""
	if journaledSend {
		if err := ctx.op.pinNotify(&message); err != nil {
			return err
		}
	}
	if kind.kind == inbox.Question {
		release, err := inbox.ReserveAnswer(dir, self.Name, message.ID)
		if err != nil {
			return failf("could not reserve the answer: %v", err)
		}
		defer release()
	}
	if err := registerGrant(dir, self, epoch, message); err != nil {
		return err
	}
	if journaledSend {
		err = ctx.op.publish(ctx, message)
	} else {
		err = writeSent(dir, self, epoch, message, session)
	}
	if err != nil {
		return err
	}
	return reportSent(ctx, sent{dir: dir, self: self, epoch: epoch, target: session, deadline: started.Add(wait), writable: grantDirs.writable}, message, kind, wait)
}

// printDelivery prints the delivery result and turns it into the exit code.
func printDelivery(ctx *Context, session registry.Session, model sendModel) error {
	line := sendLine(session, model)
	if inbox.State(model.State) != inbox.Failed && model.ID != "" {
		line += "\n" + strings.Join(append(sentGrantLines(session, model), idLines(model)...), "\n")
	}
	switch inbox.State(model.State) {
	case inbox.Delivered:
		return printValue(ctx, model, func() []string { return []string{line} })
	case inbox.Pending, inbox.Held:
		// Held is accepted and not delivered, the same promise as pending: the
		// agent has not been told, and it may yet be or not be.
		if ctx.JSON {
			_ = printValue(ctx, model, func() []string { return nil })
			return &PendingError{Message: ""}
		}
		return &PendingError{Message: line}
	default:
		if ctx.JSON {
			_ = printValue(ctx, model, func() []string { return nil })
			return &FailedError{Message: ""}
		}
		return &FailedError{Message: line}
	}
}

// idLines name the message sent: the id rewake withdraw, rewake edit and send
// --to take, and after an edit the addenda that came along to it.
func idLines(model sendModel) []string {
	lines := []string{"id " + model.ID}
	switch len(model.Addenda) {
	case 0:
	case 1:
		lines = append(lines, fmt.Sprintf("Rewake: your addendum %s now adds to %s; take it back with: rewake withdraw %s", model.Addenda[0], model.ID, shortRef(model.Addenda[0])))
	default:
		lines = append(lines, fmt.Sprintf("Rewake: your addenda %s now add to %s; take one back with: rewake withdraw <id>", strings.Join(model.Addenda, ", "), model.ID))
	}
	return lines
}

// sendLine is the one line a caller reads: what happened, by which path, and
// what it means when the answer is not "delivered".
func sendLine(session registry.Session, model sendModel) string {
	switch inbox.State(model.State) {
	case inbox.Delivered:
		line := fmt.Sprintf("Rewake: delivered to %s", session.Name)
		if model.Via != "" {
			line += " via " + model.Via
		}
		if model.Detail != "" {
			line += "; " + model.Detail
		}
		return line
	case inbox.Pending:
		return fmt.Sprintf("Rewake: pending for %s: %s", session.Name, model.Detail)
	case inbox.Held:
		return fmt.Sprintf("Rewake: held for %s: %s", session.Name, model.Detail)
	default:
		return fmt.Sprintf("Rewake: failed for %s: %s", session.Name, model.Detail)
	}
}

// waitDuration reads --wait, in seconds.
func waitDuration(call Call, fallback time.Duration) (time.Duration, error) {
	raw := call.Flag("wait", "")
	if raw == "" {
		return fallback, nil
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds < 0 {
		return 0, &UsageError{
			Command: call.Command,
			Message: fmt.Sprintf("--wait takes a number of seconds, got %q.", raw),
		}
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
