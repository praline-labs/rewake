package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
	"github.com/iiiokojiadbi/rewake/internal/state"
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

	session, err := registry.Lookup(dir, target)
	if errors.Is(err, registry.ErrNotFound) {
		return unknownSessionError(dir, target)
	}
	if err != nil {
		return &FailedError{Message: err.Error()}
	}

	if text == "-" {
		read, err := io.ReadAll(os.Stdin)
		if err != nil {
			return failf("could not read the message from stdin: %v", err)
		}
		text = string(read)
	}
	if strings.TrimSpace(text) == "" {
		return &UsageError{Command: command, Message: "The message is empty."}
	}

	kind, err := chosenKind(call)
	if err != nil {
		return err
	}
	if kind.kind == inbox.Question && role.Of(session.Role).Silent {
		return &UsageError{Command: command, Message: fmt.Sprintf("Session %s does not report its turns, so it cannot answer --question; use plain send or --notify instead.", session.Name)}
	}
	wait, err := waitDuration(call, kind.wait)
	if err != nil {
		return err
	}
	started := time.Now()

	// Signed with this session's name and run only when both are known to be
	// current: a report then reaches this run, and a process left over from an
	// earlier run cannot speak for the next one.
	self, epoch, selfErr := ownRun(dir)
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
	if kind.kind == inbox.Question {
		release, err := inbox.ReserveAnswer(dir, self.Name, message.ID)
		if err != nil {
			return failf("could not reserve the answer: %v", err)
		}
		defer release()
	}
	if err := writeSent(dir, self, epoch, message, session); err != nil {
		return err
	}
	return reportSent(ctx, sent{dir: dir, self: self, epoch: epoch, target: session, deadline: started.Add(wait), writable: grantDirs.writable}, message, kind, wait)
}

// reportSent waits for the delivery result of a message just written and
// prints it, then hands over to what its kind does next — a question waits
// for its answer.
func reportSent(ctx *Context, after sent, message inbox.Message, kind messageKind, wait time.Duration) error {
	dir, session := after.dir, after.target
	// The delivery result is worth a few seconds at most; a kind that waits
	// longer waits for something else, after it.
	status, known := awaitStatus(dir, session.Name, message.ID, min(wait, defaultWait))
	model := sendModel{ID: message.ID, To: session.Name, From: message.From, GrantGit: message.GrantGit, GrantDirs: message.GrantDirs, AlreadyWritable: after.writable, Addenda: after.addenda, Replaces: message.Replaces, Named: after.named}
	if !known && inbox.Answered(dir, session.Name, message.ID) {
		// The message left the mailbox, and a status may have been written
		// after the wait gave up. Absent a moment ago is not absent now:
		// calling a fresh delivery lost sends the sender to do it twice.
		status, known = inbox.ReadStatus(dir, session.Name, message.ID)
	}
	if known {
		model.GrantApplied = status.GrantApplied
	}
	switch {
	case known && status.State == inbox.Read:
		// Read already, which is delivered and then some.
		model.State, model.Via = string(inbox.Delivered), status.Via
		model.Detail = "already read"
	case known && status.State != inbox.Pending && status.State != inbox.Held:
		model.State, model.Via, model.Detail = string(status.State), status.Via, status.Detail
	default:
		// No final answer. Held is none either: it waits for the session that
		// holds it, and a session that is gone will never release it.
		// Promising a later delivery is only honest while the session is still
		// there to make one; a session that ended between the lookup and now
		// leaves the message with nobody to take it.
		// The name is not enough: it may already belong to a session that
		// started after this message was written, and that session will refuse
		// it. Promising a later delivery then would be a promise nobody keeps.
		current, err := registry.Lookup(dir, session.Name)
		if errors.Is(err, registry.ErrNotFound) {
			model.State = string(inbox.Failed)
			model.Detail = "the session ended before the message was delivered"
			break
		}
		if err == nil && current.Epoch() != session.Epoch() {
			model.State = string(inbox.Failed)
			model.Detail = "the session ended and another one took its name before the message was delivered"
			break
		}
		if !known && inbox.Answered(dir, session.Name, message.ID) {
			// The message is out of the mailbox but has no status: it was
			// answered long enough ago that the answer is no longer kept.
			// Saying "pending" here would promise a delivery that has happened.
			model.State = string(inbox.Failed)
			model.Detail = "this message was answered earlier and the result is no longer kept"
			break
		}
		if known && status.State == inbox.Held {
			model.State, model.Via, model.Detail = string(inbox.Held), status.Via, status.Detail
			break
		}
		model.State = string(inbox.Pending)
		if known && status.Detail != "" {
			model.Detail = status.Detail
		} else {
			model.Detail = fmt.Sprintf("no result yet after %s; the session has it and will take it", wait)
		}
	}

	if inbox.State(model.State) != inbox.Failed && kind.after != nil {
		// The id before the wait, which may be long: it is what an edit or an
		// addendum to this message takes while the sender still waits.
		if !ctx.JSON {
			_ = emit(ctx, append(writableLines(session, model), idLines(model)...)...)
		}
		after.model = model
		return kind.after(ctx, after)
	}
	return printDelivery(ctx, session, model)
}

// printDelivery prints the delivery result and turns it into the exit code.
func printDelivery(ctx *Context, session registry.Session, model sendModel) error {
	line := sendLine(session, model)
	if inbox.State(model.State) != inbox.Failed && model.ID != "" {
		line += "\n" + strings.Join(append(writableLines(session, model), idLines(model)...), "\n")
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

// awaitStatus waits for the status of a message; replaceable in tests.
var awaitStatus = inbox.Await

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
