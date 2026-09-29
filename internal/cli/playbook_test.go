package cli

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/brief"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
)

// Every step and every rule of every role has to appear in both places a
// session reads: the briefing it is launched with, and the guide it comes back
// to. One source makes the two agree in principle; this is what keeps a
// renderer from quietly dropping half of it — the review found exactly that,
// a guide that printed the steps and not the limits, with nothing failing.
//
// It lives in the cli package because it is the only one that can see both
// renderings. Checking them separately is how the gap stayed open: each test
// compared a rendering against the same slice it rendered from.
func TestBothRenderingsCarryTheWholePlaybook(t *testing.T) {
	for _, part := range role.All() {
		t.Run(part.ID, func(t *testing.T) {
			play := role.Of(part.ID).Play
			briefing := brief.Intro(brief.Context{Name: "api", Room: "work", Role: part, Reason: "selected explicitly"})
			guide := formatGuide(&play)
			for where, text := range map[string]string{"the briefing": briefing, "the guide": guide} {
				if !strings.Contains(text, play.Heading) {
					t.Errorf("%s does not say what this role is for", where)
				}
				for _, step := range play.Steps {
					if !strings.Contains(text, step.Do) {
						t.Errorf("%s is missing the step %q", where, step.Do)
					}
				}
				for _, section := range play.Sections {
					if !strings.Contains(text, section.Title) {
						t.Errorf("%s is missing the section %q", where, section.Title)
					}
					for _, line := range section.Lines {
						// The guide wraps its lines, so the comparison is on
						// words rather than on the line as written.
						if !containsWrapped(text, line) {
							t.Errorf("%s is missing the rule %q", where, line)
						}
					}
				}
			}
		})
	}
}

// containsWrapped reports whether a text carries a sentence, ignoring where
// the renderer chose to break its lines.
func containsWrapped(text, sentence string) bool {
	return strings.Contains(strings.Join(strings.Fields(text), " "), strings.Join(strings.Fields(sentence), " "))
}

// A session copies the commands its steps name, in a project where nothing
// else says how rewake is called; a step naming a command, a verb or a flag the
// table no longer has teaches a refusal. Each "rewake <command>" in Do is
// checked with what follows it: a literal word — worktree's ls, land, finish —
// has to be one the command's Args spell, and a flag one the command takes. A
// flag in Why belongs to the step's own command too, with one exception named
// here: worktree's Why names the launches' --worktree, which a harness declares
// rather than the table.
func TestEveryStepNamesARealCommand(t *testing.T) {
	invocation := regexp.MustCompile(`rewake ([a-z][a-z-]*)([^/]*)`)
	flagWord := regexp.MustCompile(`--[a-z][a-z-]*`)
	placeholder := regexp.MustCompile(`"[^"]*"|<[^>]*>|\[[^\]]*\]|--[a-z][a-z-]*|\|`)
	whyExceptions := map[string]bool{"--worktree": true}
	for flag := range whyExceptions {
		if !launchFlag(flag) {
			t.Fatalf("%s is excepted as a launch flag, and no harness takes it", flag)
		}
	}
	for _, part := range role.All() {
		for _, step := range part.Play.Steps {
			var named []*Command
			for _, match := range invocation.FindAllStringSubmatch(step.Do, -1) {
				command := findCommand(match[1])
				if command == nil {
					t.Errorf("%s: the step %q names rewake %s, which the table does not have", part.ID, step.Do, match[1])
					continue
				}
				named = append(named, command)
				spelled := strings.Fields(placeholder.ReplaceAllString(command.Args, " "))
				for _, word := range strings.Fields(placeholder.ReplaceAllString(match[2], " ")) {
					if !slices.Contains(spelled, word) {
						t.Errorf("%s: the step %q names rewake %s %s, which its Args %q do not spell", part.ID, step.Do, command.Name, word, command.Args)
					}
				}
				for _, flag := range flagWord.FindAllString(match[2], -1) {
					if !knownFlag(command, strings.TrimPrefix(flag, "--")) {
						t.Errorf("%s: the step %q names %s, which rewake %s does not take", part.ID, step.Do, flag, command.Name)
					}
				}
			}
			for _, flag := range flagWord.FindAllString(step.Why, -1) {
				if whyExceptions[flag] {
					continue
				}
				if len(named) != 1 || !knownFlag(named[0], strings.TrimPrefix(flag, "--")) {
					t.Errorf("%s: the step %q explains %s, which its own command does not take", part.ID, step.Do, flag)
				}
			}
		}
	}
}

// launchFlag reports whether a harness takes this flag at launch as rewake's own.
func launchFlag(flag string) bool {
	for _, h := range harness.All() {
		for _, single := range h.SingleUseFlags() {
			if slices.Contains(single.Spellings, flag) {
				return true
			}
		}
	}
	return false
}
