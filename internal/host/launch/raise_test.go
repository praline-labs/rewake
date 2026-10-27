package launch

import (
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// workerHarness takes its worktree flag, names a new conversation with
// --session-id and turns a template's settings into flags.
type workerHarness struct{ settingsHarness }

func (workerHarness) SingleUseFlags() []adapter.Flag {
	return append(slices.Clone(agentLike), adapter.Flag{Spellings: []string{"--effort"}, TakesValue: true})
}

func (workerHarness) Offers() adapter.Offer {
	return adapter.Offer{Conversation: adapter.ConversationOffer{Flag: "--session-id"}}
}

func (workerHarness) WorktreeFlag() string           { return "--worktree" }
func (workerHarness) WorktreeNameSpaced() bool       { return true }
func (workerHarness) WorktreeRefusal([]string) error { return nil }
func (workerHarness) LaunchDirectory(args []string) (string, []string, error) {
	return "", args, nil
}

type enteringHarness struct{ workerHarness }

func (enteringHarness) EntersWorktree() {}

var workers = adapter.NewCatalog(enteringHarness{})

func typed(value string) *string { return &value }

// The worker's worktree is resolved once, from the template and what space
// open was given, and the launch carries exactly that one answer: a typed
// --worktree, on space open or after --, wins; --no-worktree leaves the
// template's out; two typed requests, or one beside --no-worktree, are
// refused. A template that lets the worker enter a checkout does so only
// when it starts without one.
func TestRaisingResolvesTheWorktreeOnce(t *testing.T) {
	set := typesRoom(t, map[string]string{"home/.config/rewake/aliases.toml": `[alias.tree]
harness = "agent"
worktree = true

[alias.plain]
harness = "agent"

[alias.named]
harness = "agent"
args = ["--worktree=named"]

[alias.enter]
harness = "agent"
enter_worktree = true
`})
	for _, c := range []struct {
		request Request
		want    string
		enter   bool
		refused string
	}{
		{Request{Alias: "tree"}, GeneratedWorktree, false, ""},
		{Request{Alias: "tree", Worktree: typed("fix")}, "fix", false, ""},
		{Request{Alias: "tree", NoWorktree: true}, "", false, ""},
		{Request{Alias: "tree", Args: []string{"--worktree=after"}}, "after", false, ""},
		{Request{Alias: "plain"}, "", false, ""},
		{Request{Alias: "plain", Worktree: typed(GeneratedWorktree)}, GeneratedWorktree, false, ""},
		{Request{Alias: "named"}, "named", false, ""},
		{Request{Alias: "named", Worktree: typed("other")}, "other", false, ""},
		{Request{Alias: "named", NoWorktree: true}, "", false, ""},
		{Request{Alias: "enter"}, "", true, ""},
		{Request{Alias: "enter", Worktree: typed(GeneratedWorktree)}, GeneratedWorktree, false, ""},
		{Request{Alias: "plain", Worktree: typed("")}, "", false, "--worktree= needs a name"},
		{Request{Alias: "plain", Worktree: typed(GeneratedWorktree), Args: []string{"--worktree=again"}}, "", false, "given both"},
		{Request{Alias: "tree", NoWorktree: true, Worktree: typed(GeneratedWorktree)}, "", false, "ask opposite things"},
		{Request{Alias: "plain", NoWorktree: true, Args: []string{"--worktree"}}, "", false, "ask opposite things"},
		{Request{Alias: "plain", Args: []string{"--worktree", "fix-login"}}, "", false, "--worktree=fix-login"},
	} {
		raised, err := set.Raise(workers, c.request)
		if c.refused != "" {
			if err == nil || !strings.Contains(err.Error(), c.refused) {
				t.Errorf("%+v: %v, want a refusal with %q", c.request, err, c.refused)
			}
			continue
		}
		if err != nil || raised.Worktree != c.want || raised.Enter != c.enter {
			t.Errorf("%+v: worktree %q enter %v, %v; want %q, %v", c.request, raised.Worktree, raised.Enter, err, c.want, c.enter)
			continue
		}
		words, err := raised.Launch("fresh-id")
		if err != nil {
			t.Fatalf("%+v: %v", c.request, err)
		}
		var flags []string
		for _, word := range words {
			if strings.HasPrefix(word, "--worktree") {
				flags = append(flags, word)
			}
		}
		want := []string{}
		switch c.want {
		case "":
		case GeneratedWorktree:
			want = []string{"--worktree"}
		default:
			want = []string{"--worktree=" + c.want}
		}
		if !slices.Equal(flags, want) {
			t.Errorf("%+v: the launch carries %q, want %q: %q", c.request, flags, want, words)
		}
	}
}

// The worker's kind of work is resolved at space open: a typed --type wins,
// --no-type leaves the template's out, else the template's type; one that is
// not there, or that an alias file turned off, is refused before anything
// starts, saying where.
func TestRaisingResolvesTheType(t *testing.T) {
	set := typesRoom(t, map[string]string{
		"home/.config/rewake/aliases.toml": "[types]\noff = [\"process\"]\n\n[alias.coder]\nharness = \"agent\"\ntype = \"code\"\n\n" +
			"[alias.plain]\nharness = \"agent\"\n\n[alias.stale]\nharness = \"agent\"\ntype = \"process\"\n",
		"work/.rewake/types/study.md": typeFile("ours", "the project's study"),
	})
	for _, c := range []struct {
		request       Request
		want, refused string
	}{
		{Request{Alias: "coder"}, "code", ""},
		{Request{Alias: "coder", Type: typed("study")}, "study", ""},
		{Request{Alias: "coder", NoType: true}, "", ""},
		{Request{Alias: "plain"}, "", ""},
		{Request{Alias: "plain", Type: typed("review")}, "review", ""},
		{Request{Alias: "plain", Type: typed("process")}, "", "turned off in the user alias file"},
		{Request{Alias: "plain", Type: typed("nothing")}, "", `no type "nothing"`},
		{Request{Alias: "plain", Type: typed("")}, "", "--type needs a name"},
		{Request{Alias: "stale"}, "", `alias "stale" names type = "process"`},
		{Request{Alias: "stale", NoType: true}, "", ""},
		{Request{Alias: "coder", Type: typed("study"), NoType: true}, "", "ask opposite things"},
	} {
		raised, err := set.Raise(workers, c.request)
		if c.refused != "" {
			if err == nil || !strings.Contains(err.Error(), c.refused) {
				t.Errorf("%+v: %v, want a refusal with %q", c.request, err, c.refused)
			}
			continue
		}
		if err != nil || raised.Work.Name != c.want || (c.want != "" && raised.Work.Text == "") {
			t.Errorf("%+v: %+v %v, want type %q", c.request, raised.Work, err, c.want)
		}
	}
}

// A worker comes only from a template, which names its harness and role; its
// launch is the template's with the typed words merged in and the space's
// conversation id, which a literal prompt tail cannot swallow.
func TestARaisedWorkerLaunchesItsTemplate(t *testing.T) {
	set := typesRoom(t, map[string]string{"home/.config/rewake/aliases.toml": `[alias.agent-write]
harness = "agent"
role = "write"
command = "agent-worker"

[alias.agent-write.launch]
model = "chosen"
effort = "high"
label = "Chosen"

[alias.tail]
harness = "agent"
args = ["--worktree=old", "--", "--session-id=literal"]
`})
	for _, alias := range []string{"", "nothing"} {
		if _, err := set.Raise(workers, Request{Alias: alias}); err == nil {
			t.Errorf("raised from alias %q", alias)
		}
	}
	for _, c := range []struct {
		request Request
		want    []string
	}{
		{Request{Alias: "agent-write", Args: []string{"--effort", "low"}}, []string{"--command", "agent-worker", "agent", "--model", "chosen", "--effort", "low", "--session-id", "fresh-id"}},
		// A typed checkout replaces the alias's older one.
		{Request{Alias: "tail", Worktree: typed("patch")}, []string{"agent", "--worktree=patch", "--session-id", "fresh-id", "--", "--session-id=literal"}},
	} {
		raised, err := set.Raise(workers, c.request)
		if err != nil {
			t.Fatal(err)
		}
		words, err := raised.Launch("fresh-id")
		if err != nil || !slices.Equal(words, c.want) {
			t.Errorf("%+v: %q, %v; want %q", c.request, words, err, c.want)
		}
	}
	raised, err := set.Raise(workers, Request{Alias: "agent-write"})
	if err != nil || raised.Role.ID != "write" || raised.Template != "agent-write" || raised.Label != "Chosen" {
		t.Errorf("raised %+v, %v", raised, err)
	}
	raised, err = set.Raise(workers, Request{Alias: "agent-write", Args: []string{"--session-id", "mine"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raised.Launch("fresh-id"); err == nil || !strings.Contains(err.Error(), "rewake chooses the worker's --session-id") {
		t.Fatalf("a typed conversation id: %v", err)
	}
}
