package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// questionKind is a task its sender waits for: send blocks until the session
// ends its turn and prints its final message.
var questionKind = messageKind{
	kind:         inbox.Question,
	flag:         Option{Flag: "--question", Summary: "Wait for the answer: block until the session ends its turn, and print its final message."},
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

	_, found, err := inbox.AwaitAnswer(wait, question.dir, question.self.Name, question.epoch, model.ID, func(answer inbox.Message) error {
		model.State, model.Answer = string(inbox.Read), answer.Text
		return printValue(ctx, model, func() []string {
			return []string{fmt.Sprintf("answer from %s:", target.Name), answer.Text}
		})
	})
	if err != nil {
		return failf("asked %s, but its answer could not be handed over: %v", target.Name, err)
	}
	if !found {
		model.State = string(inbox.Pending)
		model.Detail = fmt.Sprintf("no answer from %s yet; it will arrive as a \"rewake: %s finished\" line, to be read with rewake inbox", target.Name, target.Name)
		line := fmt.Sprintf("asked %s: %s", target.Name, model.Detail)
		if ctx.JSON {
			_ = printValue(ctx, model, func() []string { return nil })
			return &PendingError{Message: ""}
		}
		return &PendingError{Message: line}
	}

	return nil
}
