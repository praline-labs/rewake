// Package claude describes Claude Code as a rewake harness.
package claude

import "github.com/iiiokojiadbi/rewake/internal/harness"

// ID is the launch command and the harness field of a session record.
const ID = "claude"

type claudeHarness struct{}

// New returns the Claude Code harness descriptor.
func New() harness.Harness { return claudeHarness{} }

func (claudeHarness) ID() string    { return ID }
func (claudeHarness) Title() string { return "Claude Code" }

func (claudeHarness) Summary() string {
	return "Start Claude Code as a rewake session. Messages reach it in seconds."
}

func (claudeHarness) Examples() []string {
	return []string{
		"rewake claude",
		"rewake --name api claude --model haiku",
	}
}

func (claudeHarness) Notes() []string {
	return []string{
		"Delivery goes through the session inbox socket, so a message arrives within seconds and wakes an idle session.",
		"Arguments after the harness name are passed to claude untouched.",
	}
}
