package cli

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

type greetingOptionsHarness struct{ seen harness.LaunchRequest }

func (*greetingOptionsHarness) ID() string         { return "greeting-fixture" }
func (*greetingOptionsHarness) Title() string      { return "Fixture" }
func (*greetingOptionsHarness) Summary() string    { return "Fixture" }
func (*greetingOptionsHarness) Examples() []string { return nil }
func (*greetingOptionsHarness) Notes() []string    { return nil }
func (h *greetingOptionsHarness) Launch(r harness.LaunchRequest) (harness.LaunchPlan, error) {
	h.seen = r
	return harness.LaunchPlan{Command: "/bin/sh", Args: []string{"-c", "exit 0"}}, nil
}

func (*greetingOptionsHarness) Deliver(context.Context, registry.Session, inbox.Message) inbox.Result {
	return inbox.Result{State: inbox.Delivered}
}

func TestLaunchKeepsGreetingIndependentOfSystemIntro(t *testing.T) {
	for _, flag := range []string{"no-intro", "no-greeting"} {
		t.Run(flag, func(t *testing.T) {
			t.Setenv(state.DirEnv, filepath.Join(t.TempDir(), "state"))
			fake := &greetingOptionsHarness{}
			call := Call{Command: findCommand("codex"), Flags: map[string]string{flag: "true"}}
			if err := handleLaunch(fake)(&Context{Stdout: io.Discard, Stderr: io.Discard}, call); err != nil {
				t.Fatal(err)
			}
			if fake.seen.Greeting != (flag == "no-intro") || fake.seen.Intro != (flag == "no-greeting") {
				t.Fatalf("switch %s crossed layers", flag)
			}
		})
	}
}
