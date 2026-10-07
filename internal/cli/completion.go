package cli

import (
	"context"
	"errors"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

// ReportCompletion publishes a captured backend result without sampling newer reads.
func ReportCompletion(ctx context.Context, dir string, self registry.Session, result harness.Completion) error {
	if result.Boundary == nil {
		return errors.New("completion lacks its captured read boundary; report remains pending")
	}
	return completeTurnContext(ctx, dir, self, turnEndOf(result), result.Thread)
}

// ConfirmCompletion runs an end the adapter can hold open: it publishes it and
// answers "", or holds it, keeps its answer and answers the reason the adapter
// continues the turn with (docs/v2/design-api.md#turnboundary). The end must
// name its event: a held end confirmed again — its answer lost — is known by
// it, and answered the same.
func ConfirmCompletion(ctx context.Context, dir string, self registry.Session, result harness.Completion) (string, error) {
	if result.ID == "" || result.Boundary == nil {
		return "", errors.New("a confirmed turn end names its event and its captured read boundary; report remains pending")
	}
	return endTurnContext(ctx, dir, self, turnEndOf(result), true, result.Thread)
}

func turnEndOf(result harness.Completion) inbox.TurnEnd {
	return inbox.TurnEnd{ID: result.ID, Text: result.Text, Failed: result.Kind == inbox.Error, Stopped: result.Kind == inbox.Stopped, Boundary: result.Boundary, Started: result.Started, Ended: result.Ended}
}
