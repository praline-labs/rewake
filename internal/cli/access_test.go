package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// formCase is one parsed form and the class
// docs/v2/stage3-state.md#what-is-a-writer gives it.
type formCase struct {
	argv []string
	want access
}

// documentedForms are the forms of the command table with their classes, as
// the plan's table states them. The expectation is written here, not derived
// from the classifier, so a form classified otherwise than the plan says
// fails rather than redefining the plan.
func documentedForms(launch string) []formCase {
	view, change := accessView, accessChange
	forms := []formCase{
		{nil, view},
		{[]string{"--json"}, view},
		{[]string{"guide"}, view},
		{[]string{"guide", "--json"}, view},
		{[]string{"--help"}, view},
		{[]string{"send", "--help"}, view},
		{[]string{"inbox", "--help", "--json"}, view},
		{[]string{"--version"}, view},
		{[]string{"--version", "--json"}, view},
		{[]string{"list"}, view},
		{[]string{"list", "--json"}, view},
		{[]string{"whoami"}, view},
		{[]string{"whoami", "--json"}, view},
		{[]string{"inbox", "--owed"}, view},
		{[]string{"inbox", "--awaited", "--json"}, view},
		{[]string{"worktree", "ls"}, view},
		{[]string{"worktree", "ls", "--json"}, view},
		// Refused by the handler before any effect.
		{[]string{"worktree", "prune"}, view},
		{[]string{"worktree"}, view},
		{[]string{"inbox", "--owed", "--peek"}, view},

		{[]string{launch}, change},
		{[]string{"--room", "work", "--name", "api", launch}, change},
		{[]string{"send", "api", "hello"}, change},
		{[]string{"withdraw", "8d4ddd85"}, change},
		{[]string{"edit", "8d4ddd85", "new text"}, change},
		{[]string{"inbox"}, change},
		{[]string{"inbox", "--json"}, change},
		{[]string{"inbox", "--message", "8d4ddd85"}, change},
		{[]string{"inbox", "--peek"}, change},
		{[]string{"inbox", "--next", "0123456789abcdef01234567.0.1"}, change},
		{[]string{"pending", "the review"}, change},
		{[]string{"retry", "0123456789abcdef01234567"}, change},
		{[]string{"compact", "api"}, change},
		{[]string{"interrupt", "api"}, change},
		{[]string{"worktree", "rm", "fix-login"}, change},
		{[]string{"worktree", "land", "fix-login", "--into", "release"}, change},
		{[]string{"worktree", "finish", "fix-login"}, change},
	}
	for _, kind := range sendKinds {
		if kind.flag.Flag != "" {
			forms = append(forms, formCase{[]string{"send", "api", "hello", kind.flag.Flag}, change})
		}
	}
	return forms
}

// TestEveryParsedFormHasAccessClass holds the classification of forms: every
// command carries a classifier, every documented form gets the class the plan
// gives it, every command of the table is among the documented forms, and
// every example the table prints classifies — so a command added without a
// class, or a form whose class drifts, fails here.
func TestEveryParsedFormHasAccessClass(t *testing.T) {
	table := testTable()
	launch := table.catalog.IDs()[0]

	covered := map[string]bool{}
	for _, form := range documentedForms(launch) {
		result, err := table.parse(form.argv)
		if err != nil {
			t.Errorf("%q does not parse: %v", form.argv, err)
			continue
		}
		if result.Call.Command != nil {
			covered[result.Call.Command.Name] = true
		}
		if got := formAccess(result); got != form.want {
			t.Errorf("%q is a %s, want a %s", form.argv, got, form.want)
		}
	}

	for _, group := range table.groups {
		for _, command := range group.Commands {
			if command.Access == nil {
				t.Errorf("%s has no access class", command.Name)
				continue
			}
			if !covered[command.Name] {
				t.Errorf("%s is in the table but no documented form names it; add its forms to documentedForms", command.Name)
			}
			for _, example := range command.Examples {
				words := strings.Fields(example)[1:]
				result, err := table.parse(words)
				if err != nil {
					continue // the examples' own test reports it
				}
				if got := formAccess(result); got != accessView && got != accessChange {
					t.Errorf("the example %q has no class", example)
				}
			}
		}
	}
}

// TestEveryToolCallIsAChange holds D2: through the tool every form is a
// change, its help included, because its bounded answer may be kept for
// continuing.
func TestEveryToolCallIsAChange(t *testing.T) {
	if toolAccess != accessChange {
		t.Fatalf("a tool call is a %s, want a change", toolAccess)
	}
}

// TestViewsLeaveADeadRecordInPlace holds the repaired view paths: whoami,
// inbox --owed and --awaited, list and the guide, asked by a session whose
// record is dead, look it up without pruning it; a change form asked the same
// way prunes it, which shows the record was there to prune.
func TestViewsLeaveADeadRecordInPlace(t *testing.T) {
	dir := liveSession(t, "api")
	live, err := registry.Load(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	dead := live
	dead.Name, dead.ServiceStart = "gone", live.ServiceStart+1
	if err := registry.Publish(dir, dead); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, "gone")
	t.Setenv(state.EpochEnv, dead.Epoch())
	path := state.SessionPath(dir, "gone")
	for _, argv := range [][]string{{"whoami"}, {"inbox", "--owed"}, {"inbox", "--awaited"}, {"list"}, {"guide"}, {"--help"}} {
		run(argv...)
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("%q pruned the dead record: %v", argv, err)
		}
	}
	run("inbox")
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a change form left the dead record: %v", err)
	}
}

// TestAViewIsNeverObserved holds that the shell's observation follows the
// class: after a write, a view writes no note even for a command whose change
// forms are observed, and the same call as a change does.
func TestAViewIsNeverObserved(t *testing.T) {
	dir := liveSession(t, "api")
	epoch := epochOf(t, dir, "api")
	t.Setenv(state.SessionEnv, "api")
	state.ResetReach()
	t.Cleanup(state.ResetReach)
	if err := state.WriteAtomic(filepath.Join(dir, "written"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	call := Call{Command: &Command{Name: "send"}}
	observeShell(accessView, call, nil)
	if notes := receipt.TakeShell(dir, "api", epoch); len(notes) != 0 {
		t.Fatalf("a view left %+v", notes)
	}
	observeShell(accessChange, call, nil)
	if notes := receipt.TakeShell(dir, "api", epoch); len(notes) != 1 {
		t.Fatalf("a change left %+v", notes)
	}
}
