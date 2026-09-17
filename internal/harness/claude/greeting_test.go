package claude

import (
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

func TestFreshSocketSessionsGetTheSameGuideGreeting(t *testing.T) {
	plan, err := New().Launch(harness.LaunchRequest{Name: "api", Room: "work", Dir: t.TempDir(), Greeting: true, Intro: false})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Greeting || !strings.Contains(plan.Args[len(plan.Args)-1], "Run rewake guide, read it, and reply with the single word ready.") {
		t.Fatalf("missing greeting: %q", plan.Args)
	}
	for _, args := range [][]string{{"--resume", "session"}, {"--continue"}, {"-c"}, {"-p"}, {"--debug"}, {"-p", "user prompt"}, {"--", "user prompt"}} {
		plan, err := New().Launch(harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Greeting: true, Args: args})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Greeting {
			t.Fatalf("caller input changed: %q", plan.Args)
		}
	}
}

func TestGreetingIsPositionalAfterVariadicTools(t *testing.T) {
	for _, args := range [][]string{nil, {"--"}, {"--allowedTools", "Read"}, {"--allowedTools", "Read", "--"}, {"--tools", "Read"}} {
		plan, err := New().Launch(harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Greeting: true, Args: args})
		if err != nil {
			t.Fatal(err)
		}
		if !plan.Greeting || len(plan.Args) < 2 || plan.Args[len(plan.Args)-2] != "--" {
			t.Fatalf("variadic option can consume greeting: %q", plan.Args)
		}
		separators := 0
		for _, arg := range plan.Args {
			if arg == "--" {
				separators++
			}
		}
		if separators != 1 {
			t.Fatalf("want one prompt separator, got %d: %q", separators, plan.Args)
		}
	}
}
