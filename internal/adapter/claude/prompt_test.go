package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/worktype"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The composed prompt is one argument: exactly as long as one may be, it
// goes; a byte past, it is refused, naming each part's size, rather than
// fail at exec or be cut.
func TestThePromptIsHeldToOneArgument(t *testing.T) {
	request := adapter.LaunchRequest{WorkType: worktype.Type{Name: "study", Text: "STUDY"}}
	briefing := promptPart{"rewake's briefing", strings.Repeat("b", 1000)}
	fill := maxArgument - len(briefing.text) - len("\n\n")
	if prompt, err := composePrompt([]promptPart{briefing, {"the launch's own --append-system-prompt", strings.Repeat("c", fill)}}, request); err != nil || len(prompt) != maxArgument {
		t.Fatalf("at the limit: %d bytes, %v", len(prompt), err)
	}
	_, err := composePrompt([]promptPart{briefing, {"the launch's own --append-system-prompt", strings.Repeat("c", fill+1)}}, request)
	for _, want := range []string{"1 over", "rewake's briefing 1000 bytes (the type \"study\"'s text 5 of it)", "the launch's own --append-system-prompt 130070 bytes", "--no-type"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("a byte past the limit: %v, want %q", err, want)
		}
	}
}

// The case review found: a large part of the prompt launched, and a type
// within its own bound made the whole past one argument (then the worker's
// CLAUDE.md, now the launch's own prompt). The launch refuses.
func TestATypeOverALargeProfileIsRefusedAtLaunch(t *testing.T) {
	base, err := os.MkdirTemp("", "mt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	runDir := filepath.Join(buildtest.Room(t, base, state.DefaultRoom), "runs", "api.e1")
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	for _, work := range []worktype.Type{{}, {Name: "study", Text: strings.Repeat("t", 32000)}} {
		_, err := New().Launch(adapter.LaunchRequest{
			Name: "api", Dir: t.TempDir(), Intro: true, Role: role.Write, Template: "sonnet-write",
			RunDir: runDir, WorkType: work, Args: []string{"--append-system-prompt", strings.Repeat("c", 120<<10)},
			Socket: filepath.Join(t.TempDir(), "api.sock"), Env: []string{"HOME=" + home},
		})
		if refused := err != nil && strings.Contains(err.Error(), "one argument of a launch may hold"); refused != (work.Name != "") {
			t.Errorf("type %q: %v", work.Name, err)
		}
	}
}
