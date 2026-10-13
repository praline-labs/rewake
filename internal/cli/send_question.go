package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// questionKind is a task its sender waits for: send blocks until the session
// ends its turn and prints its final message.
var questionKind = messageKind{
	kind:         mail.Question,
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
	refused := make(chan refusal, 1)
	go watchRefusal(wait, refusalWatch{dir: question.dir, to: target.Name, id: model.ID, self: question.self.Name, epoch: question.epoch}, refused, cancel)

	_, found, err := mail.AwaitAnswer(wait, question.dir, question.self.Name, question.epoch, model.ID, func(answer mail.Message) error {
		model.State, model.Answer = string(mail.Read), answer.Text
		if canSeeSessionState(question.dir) {
			model.Telemetry = sessionSnapshot(question.dir, answer.From, answer.FromEpoch)
		}
		model.ThreadChanged = answer.ThreadChanged
		model.Kind = mail.KindOf(answer)
		if model.Kind == mail.Error || model.Kind == mail.Stopped {
			model.State = string(mail.Failed)
		}
		return printValue(ctx, model, func() []string {
			heading := "answer"
			if model.Kind == mail.Error || model.Kind == mail.Stopped {
				heading = string(model.Kind)
			}
			var lines []string
			if model.Telemetry != nil {
				lines = append(lines, stateLine(answer.From, model.Telemetry, false), "")
			}
			lines = append(lines, fmt.Sprintf("%s from %s:", heading, target.Name), answer.Text)
			if answer.ThreadChanged {
				lines = append(lines, mail.ThreadChangedWarning)
			}
			return lines
		})
	})
	if err != nil {
		return failf("asked %s, but its answer could not be handed over: %v", target.Name, err)
	}
	if !found {
		select {
		case why := <-refused:
			if why.closed != nil {
				return printClosed(ctx, target, model, why.closed)
			}
			model.State, model.Via, model.Detail = string(mail.Failed), why.status.Via, why.status.Detail
			return printDelivery(ctx, target, model)
		default:
		}
		model.State = string(mail.Pending)
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

	if model.Kind == mail.Error || model.Kind == mail.Stopped {
		return &ExitCodeError{Code: ExitFailed}
	}
	return nil
}

// closedState is the state of a question a decision closed: neither
// answered nor failed in delivery.
const closedState = "closed"

// refusalPoll is how often a waiting question looks at its own status.
const refusalPoll = 250 * time.Millisecond

// refusal is why no answer is coming: the question failed, or a decision on
// the report that answers it closed the wait without proving it arrived.
type refusal struct {
	status mail.Status
	closed *mail.DecisionMark
}

// refusalWatch names the question a wait watches and the run that asked it.
type refusalWatch struct {
	dir, to, id, self, epoch string
}

// watchRefusal ends the wait for an answer once the question itself has
// failed, or once a decision closed it: the report the receiver owes was
// decided and its tail ran, so the wait is over with nothing to print.
func watchRefusal(ctx context.Context, w refusalWatch, refused chan<- refusal, stop func()) {
	ticker := time.NewTicker(refusalPoll)
	defer ticker.Stop()
	for {
		// One that cannot be read is looked at again on the next tick.
		if status, ok, err := mail.ReadStatus(w.dir, w.to, w.id); err == nil && ok && status.State == mail.Failed {
			refused <- refusal{status: status}
			stop()
			return
		}
		if mark, ok := mail.DecisionOn(w.dir, w.to, w.id, w.self, w.epoch); ok && mark.Closed {
			refused <- refusal{closed: &mark}
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

// printClosed answers a question a decision closed: no answer text, since
// none is proven to have reached this run, and none is coming.
func printClosed(ctx *Context, target registry.Session, model sendModel, mark *mail.DecisionMark) error {
	model.State, model.Decided = closedState, awaitedDecisionOf(mark)
	model.Detail = fmt.Sprintf("closed by decision %s of %s on report %s, delivery unproven: no answer is proven to have reached you, and none is coming", mark.Decision, deciderText(model.Decided.By), mark.Report)
	if mark.Resent != "" {
		model.Detail += fmt.Sprintf("; the report went again as %s and arrives as an ordinary report", mark.Resent)
	}
	if err := printValue(ctx, model, func() []string {
		return []string{fmt.Sprintf("Rewake: the question to %s was %s.", target.Name, model.Detail)}
	}); err != nil {
		return err
	}
	return &ExitCodeError{Code: ExitFailed}
}
