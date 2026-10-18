package cli

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
)

// agentID is the launch command of the adapter these tests run on.
const agentID = "agent"

// agentHarness is the adapter the CLI's tests run on. The catalog they build
// holds it alone: what the CLI does with an adapter is proven on one of its
// own, and what a product adapter does is its package's to test.
type agentHarness struct{}

func (agentHarness) ID() string      { return agentID }
func (agentHarness) Title() string   { return "Test agent" }
func (agentHarness) Summary() string { return "Start the test agent as a rewake session." }

func (agentHarness) Examples() []string {
	return []string{"rewake --main agent", "rewake --main --name api agent --model chosen", "rewake --main agent --worktree=fix-login"}
}

func (agentHarness) Notes() []string {
	return []string{"The test agent stands for any harness in the CLI's tests."}
}

func (agentHarness) SingleUseFlags() []adapter.Flag {
	return []adapter.Flag{
		{Spellings: []string{"--model"}, TakesValue: true},
		{Spellings: []string{"--worktree"}},
	}
}

func (agentHarness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	return adapter.LaunchPlan{Command: request.Program(agentID), Args: request.Args, Env: request.Env}, nil
}

// Version: no gate, so a launch in these tests reads no program's version.
func (agentHarness) Version() adapter.VersionGate { return adapter.VersionGate{} }

func (agentHarness) EnvNotInherited() []string { return nil }

// Offers: Launch alone.
func (agentHarness) Offers() adapter.Offer { return adapter.Offer{} }

func (agentHarness) WorktreeFlag() string { return "--worktree" }

// WorktreeNameSpaced: its own flag would take a name after a space, so a
// word there is refused rather than taken for the prompt.
func (agentHarness) WorktreeNameSpaced() bool { return true }

func (agentHarness) LaunchDirectory(args []string) (string, []string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", nil, err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	return resolved, args, err
}

// WorktreeRefusal: --continue would continue a conversation kept where it
// began, not in the new checkout.
func (agentHarness) WorktreeRefusal(args []string) error {
	if adapter.HasFlag(args, "--continue") {
		return errors.New("--continue continues a conversation kept elsewhere; " + adapter.ContinueInWorktree("rewake --main agent", "--continue"))
	}
	return nil
}

var _ adapter.WorktreeHarness = agentHarness{}
