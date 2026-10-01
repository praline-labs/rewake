package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
)

// questionKind is a task its sender waits for: send blocks until the session
// ends its turn and prints its final message.
var questionKind = messageKind{
	kind:         inbox.Question,
	summary:      "work whose sender waits for the outcome.",
	flag:         Option{Flag: "--question", Summary: "Block until the session ends its turn, and print its final message."},
	wait:         defaultQuestionWait,
	needsSession: "A question needs a rewake session to receive its answer, and this shell is not one; send it as a task instead, without --question",
	after:        answerQuestion,
}

// defaultQuestionWait is how long a question waits for its answer. An answer is
// a turn of the other agent, which takes minutes; a caller that expects longer
// runs the command in the background or passes --wait.
const defaultQuestionWait = 10 * time.Minute

// answerQuestion blocks until the receiver of a question ends its turn, and
// prints its last reply. The reply is taken from this session's own mailbox, so
// it is not announced to the agent a second time. Without an answer in time,
// the question stays open: the answer arrives later as an ordinary report.
func answerQuestion(ctx *Context, question sent) error {
	target, model := question.target, question.model
	wait, cancel := context.WithDeadline(context.Background(), question.deadline)
	defer cancel()
	// A question can still fail after send reported it held or pending — the
	// hold expired, the session ended — and then no answer is coming. Waiting
	// out the deadline for one would block the asker for nothing.
	refused := make(chan inbox.Status, 1)
	go watchRefusal(wait, question.dir, target.Name, model.ID, refused, cancel)

	_, found, err := inbox.AwaitAnswer(wait, question.dir, question.self.Name, question.epoch, model.ID, func(answer inbox.Message) error {
		model.State, model.Answer = string(inbox.Read), answer.Text
		if canSeeSessionState(question.dir) {
			model.Telemetry = sessionSnapshot(question.dir, answer.From, answer.FromEpoch)
		}
		model.ThreadChanged = answer.ThreadChanged
		model.Kind = inbox.KindOf(answer)
		if model.Kind == inbox.Error || model.Kind == inbox.Stopped {
			model.State = string(inbox.Failed)
		}
		return printValue(ctx, model, func() []string {
			heading := "answer"
			if model.Kind == inbox.Error || model.Kind == inbox.Stopped {
				heading = string(model.Kind)
			}
			var lines []string
			if model.Telemetry != nil {
				lines = append(lines, stateLine(answer.From, model.Telemetry, false), "")
			}
			lines = append(lines, fmt.Sprintf("%s from %s:", heading, target.Name), answer.Text)
			if answer.ThreadChanged {
				lines = append(lines, inbox.ThreadChangedWarning)
			}
			return lines
		})
	})
	if err != nil {
		return failf("asked %s, but its answer could not be handed over: %v", target.Name, err)
	}
	if !found {
		select {
		case status := <-refused:
			model.State, model.Via, model.Detail = string(inbox.Failed), status.Via, status.Detail
			return printDelivery(ctx, target, model)
		default:
		}
		model.State = string(inbox.Pending)
		model.Detail = fmt.Sprintf("no answer from %s yet; it will arrive as a \"Rewake: %s finished\" line or a grouped notice, to be read with rewake inbox", target.Name, target.Name)
		// The detail stays as it was for --json; the line leaves out how the
		// answer is announced, which the guide says.
		line := fmt.Sprintf("Rewake: no answer from %s yet; it arrives later as its report", target.Name)
		if ctx.JSON {
			_ = printValue(ctx, model, func() []string { return nil })
			return &PendingError{Message: ""}
		}
		return &PendingError{Message: line}
	}

	if model.Kind == inbox.Error || model.Kind == inbox.Stopped {
		return &ExitCodeError{Code: ExitFailed}
	}
	return nil
}

// refusalPoll is how often a waiting question looks at its own status.
const refusalPoll = 250 * time.Millisecond

// watchRefusal ends the wait for an answer once the question itself has failed.
func watchRefusal(ctx context.Context, dir, to, id string, refused chan<- inbox.Status, stop func()) {
	ticker := time.NewTicker(refusalPoll)
	defer ticker.Stop()
	for {
		// One that cannot be read is looked at again on the next tick.
		if status, ok, err := inbox.ReadStatus(dir, to, id); err == nil && ok && status.State == inbox.Failed {
			refused <- status
			stop()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
