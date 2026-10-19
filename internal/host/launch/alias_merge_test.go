package launch

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// agentLike stands in for what a harness publishes about its own flags: the
// ones it takes at most once, each with all its spellings. Anything absent is
// appended rather than replaced.
var agentLike = []adapter.Flag{
	{Spellings: []string{"--model", "-m"}, TakesValue: true},
	{Spellings: []string{"--sandbox", "-s"}, TakesValue: true},
	{Spellings: []string{"--search"}},
	{Spellings: []string{"--worktree"}},
}

// Each case is a defect seen before, in the direct alias launch these rules
// came from: the arguments that come out, not only their order, since a
// harness may refuse a repeated single-use flag outright.
func TestTypedArgumentsMergeWithTheAlias(t *testing.T) {
	model := []string{"--model", "a-model", "-c", "key=value"}
	for _, tc := range []struct {
		name         string
		alias, typed []string
		want         string
	}{
		{"a typed flag replaces the alias copy", model, []string{"--model", "another"}, "-c key=value --model another"},
		// A repeated -c merges a structured value; only the harness can tell.
		{"settings pass through untouched", []string{"-c", "effort=high", "-c", "sandbox=off"}, []string{"-c", "effort=low"}, "-c effort=high -c sandbox=off -c effort=low"},
		{"an accumulating flag keeps both", []string{"--add-dir", "/one", "--enable", "a-feature"}, []string{"--add-dir", "/two"}, "--add-dir /one --enable a-feature --add-dir /two"},
		{"a short form replaces the long one", model, []string{"-m", "typed"}, "-c key=value -m typed"},
		{"an unlisted flag is appended", []string{"--unlisted", "one"}, []string{"--unlisted", "two"}, "--unlisted one --unlisted two"},
		{"replacing a switch keeps what follows it", []string{"--search", "the prompt"}, []string{"--search"}, "the prompt --search"},
		{"a typed --worktree replaces the alias's", []string{"--worktree=from-alias"}, []string{"--worktree=typed"}, "--worktree=typed"},
		// After "--" a word that looks like a flag is a prompt's text.
		{"a terminator on the line ends replacement", model, []string{"x", "--", "--model", "words"}, "--model a-model -c key=value x -- --model words"},
		{"a terminator in the alias ends replacement", []string{"--", "--model", "p"}, []string{"--model", "typed"}, "-- --model p --model typed"},
		{"a terminator in the alias ends the typed tail too", []string{"--model", "a-model", "--"}, []string{"--model"}, "--model a-model -- --model"},
		{"terminators on both sides leave everything", []string{"--model", "a-model", "--", "first"}, []string{"--sandbox", "ro", "--", "--model"}, "--model a-model -- first --sandbox ro -- --model"},
		{"a joined short form after a terminator is input", []string{"-ma-model", "--"}, []string{"-mtyped"}, "-ma-model -- -mtyped"},
		{"a joined short form on the line replaces", model, []string{"-mtyped"}, "-c key=value -mtyped"},
		{"a joined short form in the alias is replaced", []string{"-ma-model", "--sandbox", "ro"}, []string{"--model", "typed"}, "--sandbox ro --model typed"},
	} {
		if got := strings.Join(MergeArguments(tc.alias, tc.typed, agentLike), " "); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
