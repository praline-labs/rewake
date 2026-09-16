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
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// defaultWait is how long send waits for a result before reporting what it
// knows. It is short because the answer for a healthy delivery arrives in well
// under a second, and a caller that has to wait longer wants to hear why.
const defaultWait = 5 * time.Second

// sendModel is the machine form of one send.
type sendModel struct {
	ID     string `json:"id"`
	To     string `json:"to"`
	From   string `json:"from"`
	State  string `json:"state"`
	Via    string `json:"via,omitempty"`
	Detail string `json:"detail,omitempty"`
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

	wait, err := waitDuration(call)
	if err != nil {
		return err
	}

	// Reading stdin can take a while, and a session can end in that time. The
	// epoch pins the message to this run of the name, so a later session that
	// takes the same name does not receive somebody else's mail.
	if _, err := registry.Lookup(dir, session.Name); errors.Is(err, registry.ErrNotFound) {
		return unknownSessionError(dir, target)
	}

	kind := inbox.Note
	if call.Switch("question") {
		kind = inbox.Question
	}
	message := inbox.Message{
		ID:        inbox.NewID(),
		From:      harness.ShellSender,
		To:        session.Name,
		ToEpoch:   session.Epoch(),
		Kind:      kind,
		Text:      text,
		CreatedAt: time.Now(),
	}
	// Signed with this session's name and run only when both are known to be
	// current: an answer then reaches this run, and a process left over from an
	// earlier run cannot speak for the next one.
	self, epoch, selfErr := ownRun(dir)
	peer := inbox.Waiter{Name: session.Name, Epoch: session.Epoch()}
	if selfErr == nil {
		message.From, message.FromEpoch = self.Name, epoch
		// Writing to a session that waits for this turn answers it: the answer
		// says more than a notice that the turn ended, and reading it must not
		// ask for a notice back.
		message.Reply = inbox.Awaits(dir, self.Name, epoch, peer)
	}
	if err := inbox.Put(dir, message); err != nil {
		return failf("could not write the message into the mailbox of %s: %v", session.Name, err)
	}
	if message.Reply {
		// Forgotten only once the answer is written: a failed write leaves the
		// report owed.
		inbox.ClearAwaiting(dir, self.Name, epoch, peer.Name)
	}

	status, known := inbox.Await(dir, session.Name, message.ID, wait)
	model := sendModel{ID: message.ID, To: session.Name, From: message.From}
	switch {
	case known && status.State == inbox.Read:
		// Read already, which is delivered and then some.
		model.State, model.Via = string(inbox.Delivered), status.Via
		model.Detail = "already read"
	case known && status.State != inbox.Pending:
		model.State, model.Via, model.Detail = string(status.State), status.Via, status.Detail
	default:
		// No final answer. Promising a later delivery is only honest while the
		// session is still there to make one; a session that ended between the
		// lookup and now leaves the message with nobody to take it.
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
		model.State = string(inbox.Pending)
		if known && status.Detail != "" {
			model.Detail = status.Detail
		} else {
			model.Detail = fmt.Sprintf("no result yet after %s; the session has it and will take it", wait)
		}
	}

	line := sendLine(session, model)
	switch inbox.State(model.State) {
	case inbox.Delivered:
		return printValue(ctx, model, func() []string { return []string{line} })
	case inbox.Pending:
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

// sendLine is the one line a caller reads: what happened, by which path, and
// what it means when the answer is not "delivered".
func sendLine(session registry.Session, model sendModel) string {
	switch inbox.State(model.State) {
	case inbox.Delivered:
		line := fmt.Sprintf("delivered to %s", session.Name)
		if model.Via != "" {
			line += " via " + model.Via
		}
		if model.Detail != "" {
			line += "; " + model.Detail
		}
		return line
	case inbox.Pending:
		return fmt.Sprintf("pending for %s: %s", session.Name, model.Detail)
	default:
		return fmt.Sprintf("failed for %s: %s", session.Name, model.Detail)
	}
}

// waitDuration reads --wait, in seconds.
func waitDuration(call Call) (time.Duration, error) {
	raw := call.Flag("wait", "")
	if raw == "" {
		return defaultWait, nil
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
