package brief

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
)

// Main's briefing states the authorship policy and where it came from, and the
// playbook it renders is the one the policy chooses: a session should not have
// to open the alias file to learn why it may or may not implement.
func TestMainsBriefingSaysItsAuthorshipPolicyAndSource(t *testing.T) {
	base := Context{Name: "api", Room: "work", Role: role.Main, Reason: "selected explicitly"}
	for _, tc := range []struct {
		name string
		c    Context
		want []string
		bad  []string
	}{
		{"default", base, []string{"no project sets [main] implements, the default", "Do not write product code yourself"}, nil},
		{"explicitly delegating", withPolicy(base, false, true), []string{"the project's [main] implements = false", "Do not write product code yourself"}, []string{"no project sets"}},
		{"implementing", withPolicy(base, true, true), []string{"lets main implement product changes itself", "may implement product changes yourself"}, []string{"do not write product code yourself", "no project sets"}},
	} {
		text := Intro(tc.c)
		for _, want := range tc.want {
			if !strings.Contains(text, want) {
				t.Errorf("%s: missing %q", tc.name, want)
			}
		}
		for _, bad := range tc.bad {
			if strings.Contains(text, bad) {
				t.Errorf("%s: carries %q", tc.name, bad)
			}
		}
	}
	// A worker is not told about main's policy at all.
	worker := Intro(Context{Name: "w", Room: "work", Role: role.Write, MainImplements: true, MainPolicyExplicit: true})
	if strings.Contains(worker, "Main implementation") || strings.Contains(worker, "main-implementation policy") {
		t.Errorf("a worker's briefing carries main's policy: %s", worker)
	}
}

// The mode section is the same policy: an implementing main is not told in
// build to delegate, and keeps the read-only discipline of advisor and review.
func TestMainsModeSectionFollowsTheSamePolicy(t *testing.T) {
	for _, name := range []string{workmode.Build, workmode.Advisor, workmode.Review, workmode.Process} {
		mode := workmode.BuiltIns()[name]
		text := Intro(Context{Name: "api", Room: "work", Role: role.Main, Mode: mode, MainImplements: true})
		if !strings.Contains(text, mode.Text) || !strings.Contains(text, "implementation is permitted only when the task, the mode and the project's rules allow it") {
			t.Errorf("%s: %s", name, text)
		}
		if strings.Contains(text, "do not write product code yourself") {
			t.Errorf("%s: the delegation prohibition survived", name)
		}
		delegating := Intro(Context{Name: "api", Room: "work", Role: role.Main, Mode: mode})
		if !strings.Contains(delegating, "do not write product code yourself") {
			t.Errorf("%s: the default lost the prohibition", name)
		}
	}
}

func withPolicy(c Context, implements, explicit bool) Context {
	c.MainImplements, c.MainPolicyExplicit = implements, explicit
	return c
}
