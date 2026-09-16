// Package codex describes Codex CLI as a rewake harness.
package codex

import (
	"context"
	"errors"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

// ID is the launch command and the harness field of a session record.
const ID = "codex"

type codexHarness struct{}

// New returns the Codex harness descriptor.
func New() harness.Harness { return codexHarness{} }

func (codexHarness) ID() string    { return ID }
func (codexHarness) Title() string { return "Codex" }

func (codexHarness) Summary() string {
	return "Start Codex as a rewake session. Messages reach it within about ten seconds."
}

func (codexHarness) Examples() []string {
	return []string{
		"rewake codex",
		"rewake --name web codex --model gpt-5.6-terra",
	}
}

func (codexHarness) Notes() []string {
	return []string{
		"Codex polls for queued messages every ten seconds, so delivery is not instant; the send command says so in its result.",
		"A Codex session that has not exchanged a single message yet cannot accept one: such a message stays pending and lands after its first turn.",
		"Arguments after the harness name are passed to codex untouched.",
	}
}

// Launch and Deliver land in milestone 4, together with the live check that
// codex queue reaches a running TUI. Until then the harness is listed and
// documented but refuses to start, which is the honest state of it.
func (codexHarness) Launch(harness.LaunchRequest) (harness.LaunchPlan, error) {
	return harness.LaunchPlan{}, errors.New("running codex under rewake is not built yet")
}

func (codexHarness) Deliver(context.Context, registry.Session, inbox.Message) inbox.Result {
	return inbox.Result{State: inbox.Failed, Detail: "codex delivery is not built yet"}
}
