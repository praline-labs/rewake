package pi

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
)

// A pi run's briefing carries the role and then the APPEND_SYSTEM.md pi would
// have found itself, which the briefing's flag turns off: the project's when
// trusted, else the agent directory's; none when the caller's own flag turned
// discovery off, as pi would. It carries no work mode: pi's extension injects
// that one section per request, so a switch replaces it rather than stack
// against the launch (the defect this guards).
func TestABriefingKeepsWhatPiWouldAppend(t *testing.T) {
	write := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("appended"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	work := t.TempDir()
	t.Chdir(work)
	for _, c := range []struct {
		args            []string
		project, global bool
		want            string
	}{
		{nil, true, true, filepath.Join(work, ".pi", "APPEND_SYSTEM.md")},
		{[]string{"-na"}, true, true, "global"},
		// The last trust switch wins, as in pi: an alias's --no-approve with
		// --approve typed after it trusts the project.
		{[]string{"--no-approve", "--approve"}, true, true, filepath.Join(work, ".pi", "APPEND_SYSTEM.md")},
		{[]string{"--approve", "-na"}, true, true, "global"},
		{nil, false, true, "global"},
		{nil, false, false, ""},
		{[]string{"--append-system-prompt", "mine"}, true, true, ""},
	} {
		request := launchRequest(t)
		request.Args, request.Intro, request.Role = c.args, true, role.Write
		request.Mode = workmode.Mode{Name: "study", Text: "STUDY-TEXT", Source: workmode.User}
		_ = os.RemoveAll(filepath.Join(work, ".pi"))
		if c.project {
			write(filepath.Join(work, ".pi", "APPEND_SYSTEM.md"))
		}
		global := filepath.Join(os.Getenv("HOME"), ".config", "rewake", "pi", "agent", "APPEND_SYSTEM.md")
		if c.global {
			write(global)
		}
		if c.want == "global" {
			c.want = global
		}
		plan, err := harness{}.Launch(request)
		if err != nil {
			t.Fatal(err)
		}
		appended := adapter.FlagValues(plan.Args, appendFlag)
		ours := slices.IndexFunc(appended, func(text string) bool { return strings.Contains(text, `session "api"`) })
		if ours < 0 {
			t.Fatalf("%v: no briefing in %q", c.args, appended)
		}
		if strings.Contains(appended[ours], "STUDY-TEXT") || strings.Contains(appended[ours], "YOUR KIND OF WORK") || strings.Contains(appended[ours], "Runtime work mode") {
			t.Fatalf("%v: the launch briefing carries a work mode, which the extension injects per request: %q", c.args, appended[ours])
		}
		var after []string
		if ours+1 < len(appended) {
			after = appended[ours+1:]
		}
		if (c.want == "") != (len(after) == 0) || (c.want != "" && !slices.Equal(after, []string{c.want})) {
			t.Errorf("%v project %v global %v: after the briefing %q, want %q", c.args, c.project, c.global, after, c.want)
		}
	}
}
