package codex

import (
	"strings"
	"testing"
)

// A continuation is refused with --worktree however Codex lets it be spelled.
// The six forms a parser following the grammar missed are here: an image's
// joined value ends in its own argument, so the word after it is the
// subcommand; a prompt may come before the subcommand; and an option the
// parser did not know hid the word after its value. Each was seen accepted as
// a continuation by the 0.155.1 terminal.
func TestAWorktreeRefusesEveryContinuationForm(t *testing.T) {
	for _, args := range [][]string{
		{"resume"},
		{"fork", "--last"},
		{"-m", "test", "resume", "--last", "next prompt"},
		{"--cd=/tmp", "fork", "0199"},
		{"--image", "foo", "--model", "test", "resume"},
		{"--image=foo", "resume"},
		{"-ifoo", "fork"},
		{"-i=foo", "resume"},
		{"caller prompt", "resume"},
		{"caller prompt", "fork"},
		{"--remote-auth-token-env", "TOKEN", "resume"},
		{"resume", "--", "fork"},
	} {
		err := (codexHarness{}).WorktreeRefusal(args)
		if err == nil || !strings.Contains(err.Error(), "Start a new conversation with --worktree") {
			t.Errorf("%q: %v, want the continuation refused", args, err)
		}
	}
}

// Only the arguments before the terminator are Codex's to parse; after it the
// word is the prompt's text and starts a new conversation.
func TestAWorktreeTakesAPromptAfterTheTerminator(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"--", "resume"},
		{"-m", "test", "--", "fork", "--last"},
		{"resumed work"},
		{"--image", "resume.png"},
	} {
		if err := (codexHarness{}).WorktreeRefusal(args); err != nil {
			t.Errorf("%q refused: %v", args, err)
		}
	}
}
