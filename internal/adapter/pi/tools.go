package pi

import (
	"context"
	"encoding/json"
	"time"

	"github.com/praline-labs/rewake/internal/core/sessiontools"
)

// rewake's tools as data (docs/worktree-tools.md): the table goes to the
// extension in the hello's answer, and once more after an adoption, since the
// next image may offer another; a call is planned here and run by the
// extension through pi's exec, with the call's abort, and its raw end is
// worded here. Nothing of a call is kept: a plan or a result read while a
// handover holds the reader waits in the socket for the next image, which
// answers it alike.

// toolEntry is one tool as the extension registers it.
type toolEntry struct {
	Name        string          `json:"name"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// table is the tools the run's role is offered: a pi run enters no checkout
// by a tool.
func (r *run) table() []toolEntry {
	tools := sessiontools.Offered(r.tools.Role, false)
	entries := make([]toolEntry, 0, len(tools))
	for _, tool := range tools {
		entries = append(entries, toolEntry{Name: tool.Name, Label: tool.Label, Description: tool.Description, InputSchema: tool.InputSchema})
	}
	return entries
}

// toolPlan answers a call's argv and bound, or the refusal the model reads.
// pi's exec takes no environment: the command inherits pi's, which the
// launch gave the run's variables the plan names.
func (r *run) toolPlan(request frame) frame {
	var input map[string]any
	if err := json.Unmarshal(request.Arguments, &input); err != nil || input == nil {
		return frame{Refused: request.Tool + "'s arguments are no JSON object"}
	}
	plan, refused := r.tools.Plan(request.Tool, input)
	if refused != "" {
		return frame{Refused: refused}
	}
	return frame{Argv: plan.Argv, TimeoutMs: plan.TimeoutMs}
}

// toolResult words a call's raw end.
func (r *run) toolResult(request frame) frame {
	if request.Ran == nil {
		return frame{Error: "a tool's result names how its command ended"}
	}
	outcome := sessiontools.Answer(request.Tool, *request.Ran)
	return frame{Text: outcome.Text, IsError: outcome.IsError}
}

// sendTools gives an adopted connection this image's table: the extension
// registers what it adds or changes, and hides what it drops. Unanswered, the
// extension keeps the table it holds, whose calls this image still plans.
func (r *run) sendTools(l *link, thread string) {
	ctx, cancel := context.WithTimeout(r.ctx, 5*time.Second)
	defer cancel()
	_, _ = l.ask(ctx, frame{Op: "tools", Thread: thread, Tools: r.table()})
}
