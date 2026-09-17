package codex

import (
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

func TestFreshConversationsGetAGreetingWithoutLosingArguments(t *testing.T) {
	codexHome(t, "")
	for _, args := range [][]string{nil, {"--model", "chosen"}, {"--config", "key=value"}, {"--"}} {
		plan, err := New().Launch(harness.LaunchRequest{Name: "api", Room: "work", Dir: t.TempDir(), Role: role.General, Greeting: true, Args: args})
		if err != nil {
			t.Fatal(err)
		}
		if !plan.Greeting || !strings.Contains(plan.Args[len(plan.Args)-1], "single word ready") {
			t.Fatalf("no greeting: args=%q notes=%q", plan.Args, plan.Notes)
		}
		if len(args) > 0 && args[0] != "--" && strings.Join(plan.Args[:len(args)], "\x00") != strings.Join(args, "\x00") {
			t.Fatalf("arguments changed: %q", plan.Args)
		}
	}
}

func TestContinuationsAndUserPromptsKeepTheirInput(t *testing.T) {
	codexHome(t, "")
	for _, args := range [][]string{{"resume", "--last"}, {"fork", "thread"}, {"my prompt"}, {"--", "my prompt"}, {"--unknown", "value"}} {
		plan, err := New().Launch(harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Greeting: true, Args: args})
		if err != nil {
			t.Fatal(err)
		}
		if plan.Greeting || strings.Contains(strings.Join(plan.Args, " "), "single word ready") {
			t.Fatalf("greeting overwrote caller intent: %q", plan.Args)
		}
	}
}

func TestGreetingIsPositionalAfterVariadicImages(t *testing.T) {
	codexHome(t, "")
	for _, args := range [][]string{nil, {"--"}, {"--image", "image.png"}, {"-c", `notify=["user-notify"]`, "--image", "image.png"}, {"-iimage.png", "--"}} {
		plan, err := New().Launch(harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Role: role.General, Greeting: true, Args: args})
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
