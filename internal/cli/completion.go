package cli

import (
	"context"
	"fmt"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// ReportCompletion publishes a turn's end with the scope it captured, without
// sampling newer reads. An end without its event, its scope or a known reason
// is refused, and the waits it would answer stay owed for the next end
// (docs/rules/effects.md, E7).
func ReportCompletion(ctx context.Context, dir string, self registry.Session, end adapter.TurnEnd) error {
	if err := end.Check(); err != nil {
		return fmt.Errorf("%w; report remains pending", err)
	}
	return mail.CompleteTurn(ctx, dir, self, turnEndOf(end), end.Thread, mail.TurnEndOptions{BeforeReports: beforeReports})
}

// ConfirmCompletion runs an end the adapter can hold open: it publishes it and
// answers "", or holds it, keeps its answer and answers the reason the adapter
// continues the turn with (docs/v2/design-api.md#turnboundary). A held end
// confirmed again — its answer lost — is known by its event, and answered the
// same.
func ConfirmCompletion(ctx context.Context, dir string, self registry.Session, end adapter.TurnEnd) (string, error) {
	if err := end.Check(); err != nil {
		return "", fmt.Errorf("%w; report remains pending", err)
	}
	return mail.EndTurn(ctx, dir, self, turnEndOf(end), true, end.Thread, mail.TurnEndOptions{BeforeReports: beforeReports})
}

// turnEndOf is the core's end for an adapter's: what the end reports follows
// from its reason (docs/rules/turns.md, O2). An aborted turn reports stopped;
// a failed one an error with the harness's words unchanged, and so does a
// refused one, whose words are no answer to the work it was given.
func turnEndOf(end adapter.TurnEnd) mail.TurnEnd {
	return mail.TurnEnd{
		ID: end.Event, Text: end.Answer,
		Failed:   end.Reason == adapter.Failed || end.Reason == adapter.Refused,
		Stopped:  end.Reason == adapter.Aborted,
		Boundary: end.Boundary, Started: end.Started, Ended: end.Ended,
	}
}

// beforeReports is the BeforeReports every turn end of cli passes. It does
// nothing; a test widens that window through it.
var beforeReports = func() {}
