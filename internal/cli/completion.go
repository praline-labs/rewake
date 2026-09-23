package cli

import (
	"context"
	"errors"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// ReportCompletion publishes a captured backend result without sampling newer reads.
func ReportCompletion(ctx context.Context, dir string, self registry.Session, result harness.Completion) error {
	if result.Boundary == nil {
		return errors.New("completion lacks its captured read boundary; report remains pending")
	}
	return completeTurnContext(ctx, dir, self, turnResult{ID: result.ID, Text: result.Text, Failed: result.Kind == inbox.Error, Stopped: result.Kind == inbox.Stopped, Boundary: result.Boundary, Started: result.Started, Ended: result.Ended}, result.Thread)
}
