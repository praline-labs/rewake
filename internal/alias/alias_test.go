package alias

// What an alias expands into: the order, what a typed flag replaces, and where
// the flags of a command line end. Reading the files and refusing a bad one
// live in alias_file_test.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// A set built in memory, so a test says what it is about rather than where the
// file happened to be.
func set(t *testing.T, contents string) *Set {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, projectFile)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newSet()
	s.read(path, "the project alias file", false)
	return s
}

// valued stands in for the CLI's own knowledge of which flags take a value.
func valued(name string) bool { return name == "name" || name == "room" }

// codexLike stands in for what a harness publishes about its own flags: the
// ones it takes at most once, each with all its spellings. Anything absent is
// appended rather than replaced.
func codexLike(string) []harness.Flag {
	return []harness.Flag{
		{Spellings: []string{"--model", "-m"}, TakesValue: true},
		{Spellings: []string{"--sandbox", "-s"}, TakesValue: true},
		{Spellings: []string{"--search"}},
	}
}

const wcodex = `
[alias.wcodex]
harness = "codex"
rewake = ["--write", "--name", "writer"]
args = ["--model", "a-model", "-c", "key=value"]
`

func TestExpandPutsTheAliasBeforeWhatWasTyped(t *testing.T) {
	s := set(t, wcodex)
	got, err := s.Expand([]string{"wcodex"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--write", "--name", "writer", "codex", "--model", "a-model", "-c", "key=value"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A flag typed on the line replaces the alias's copy of it — the copy is
// dropped, not merely outranked. Ordering alone is not enough: Codex refuses a
// repeated --model outright, so leaving both would end the launch with a
// complaint about a flag the person wrote once.
//
// This checks the arguments that come out, not the order they come out in. The
// first version of it compared the order and would have passed on a command
// the harness refuses to parse.
func TestATypedFlagReplacesTheAliasCopy(t *testing.T) {
	s := set(t, wcodex)
	got, err := s.Expand([]string{"--name", "reviewer", "wcodex", "--model", "another"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	if n := count(got, "--model"); n != 1 {
		t.Fatalf("--model appears %d times: %v", n, got)
	}
	if n := count(got, "--name"); n != 1 {
		t.Fatalf("--name appears %d times: %v", n, got)
	}
	if value(got, "--model") != "another" || value(got, "--name") != "reviewer" {
		t.Fatalf("the typed values did not win: %v", got)
	}
	// Untouched: the alias said something the person did not.
	if value(got, "-c") != "key=value" || !contains(got, "--write") {
		t.Fatalf("the rest of the alias was lost: %v", got)
	}
}

// Settings are not collapsed at all. A repeated -c key takes its last value
// when the value is plain, but structured settings merge — so dropping the
// alias's earlier one can remove part of a setting the typed one does not
// restate. Telling those apart means understanding the value; the harness
// already does, so both are passed, the alias's first, and it decides.
func TestSettingsArePassedThroughUntouched(t *testing.T) {
	s := set(t, `
[alias.x]
harness = "codex"
args = ["-c", "effort=high", "-c", "sandbox=off"]
`)
	got, err := s.Expand([]string{"x", "-c", "effort=low"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(got, " "); joined != "codex -c effort=high -c sandbox=off -c effort=low" {
		t.Fatalf("settings were rearranged: %s", joined)
	}
}

// A flag that adds something rather than choosing it keeps both: a typed
// --add-dir must not remove the directory the alias named.
func TestAnAccumulatingFlagKeepsBoth(t *testing.T) {
	s := set(t, `
[alias.x]
harness = "codex"
args = ["--add-dir", "/one", "--enable", "a-feature"]
`)
	got, err := s.Expand([]string{"x", "--add-dir", "/two"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--add-dir /one") || !strings.Contains(joined, "--add-dir /two") {
		t.Fatalf("a directory was lost: %s", joined)
	}
	if !strings.Contains(joined, "--enable a-feature") {
		t.Fatalf("an unrelated flag was dropped: %s", joined)
	}
}

// One parameter, two spellings: a typed -m replaces the alias's --model,
// because the harness says they are the same thing.
func TestAShortFormReplacesTheLongOne(t *testing.T) {
	s := set(t, wcodex)
	got, err := s.Expand([]string{"wcodex", "-m", "typed"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "a-model") {
		t.Fatalf("the alias model survived a typed -m: %s", joined)
	}
	if !strings.Contains(joined, "-m typed") {
		t.Fatalf("the typed model is missing: %s", joined)
	}
}

// A flag the harness did not list is appended, which is the safe answer: we do
// not know whether two of them mean "the last wins" or "both apply".
func TestAnUnlistedFlagIsAppended(t *testing.T) {
	s := set(t, `
[alias.x]
harness = "codex"
args = ["--unlisted", "one"]
`)
	got, err := s.Expand([]string{"x", "--unlisted", "two"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(got, " "); joined != "codex --unlisted one --unlisted two" {
		t.Fatalf("an unlisted flag was treated as single-use: %s", joined)
	}
}

func count(arguments []string, flag string) int {
	seen := 0
	for _, argument := range arguments {
		if argument == flag {
			seen++
		}
	}
	return seen
}

func value(arguments []string, flag string) string {
	for i, argument := range arguments {
		if argument == flag && i+1 < len(arguments) {
			return arguments[i+1]
		}
	}
	return ""
}

// Dropping a switch must not drop what stands behind it. The expansion cannot
// tell a value from an unrelated argument by looking, so the harness says
// which of its flags carry one — and a switch carries nothing.
func TestReplacingASwitchKeepsWhatFollowsIt(t *testing.T) {
	s := set(t, `
[alias.x]
harness = "codex"
args = ["--search", "the prompt the alias carries"]
`)
	got, err := s.Expand([]string{"x", "--search"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "the prompt the alias carries") {
		t.Fatalf("the argument behind the switch was taken with it: %s", joined)
	}
	if n := count(got, "--search"); n != 1 {
		t.Fatalf("--search appears %d times: %s", n, joined)
	}
}

// The four boundary cases the acceptance review of September 21, 2026 found,
// kept as tests rather than as a note: both sides of the "--" terminator, and
// both sides of a short flag joined to its value.

// After "--" the words are input for the harness — a prompt, usually. A word
// there that looks like a flag is text, and reading it as one took the model
// out of an alias because the prompt happened to contain "--model".
func TestATerminatorOnTheLineEndsReplacement(t *testing.T) {
	s := set(t, wcodex)
	got, err := s.Expand([]string{"wcodex", "x", "--", "--model", "words of a prompt"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--model a-model") {
		t.Fatalf("a prompt after -- took the alias model away: %s", joined)
	}
}

// And the same terminator inside the alias: what it puts after "--" is the
// harness's input, not a flag that a typed one may replace.
func TestATerminatorInTheAliasEndsReplacement(t *testing.T) {
	s := set(t, `
[alias.x]
harness = "codex"
args = ["--", "--model", "a prompt the alias carries"]
`)
	got, err := s.Expand([]string{"x", "--model", "typed"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "-- --model a prompt the alias carries") {
		t.Fatalf("the alias input after -- was rewritten: %s", joined)
	}
}

// The case the two halves missed between them: the alias ends with a
// terminator, so everything the line adds is input for the harness — and a
// word of that input took a setting out of the alias.
func TestATerminatorInTheAliasEndsTheTypedTailToo(t *testing.T) {
	s := set(t, `
[alias.x]
harness = "codex"
args = ["--model", "a-model", "--"]
`)
	got, err := s.Expand([]string{"x", "--model"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--model a-model") {
		t.Fatalf("a word of the prompt replaced the alias setting: %s", joined)
	}
	if !strings.HasSuffix(joined, "-- --model") {
		t.Fatalf("the typed word did not stay input: %s", joined)
	}
}

// A terminator on both sides: the alias's comes first in the command being
// built, so it governs, and nothing is replaced.
func TestTerminatorsOnBothSidesLeaveEverythingAlone(t *testing.T) {
	s := set(t, `
[alias.x]
harness = "codex"
args = ["--model", "a-model", "--", "first prompt"]
`)
	got, err := s.Expand([]string{"x", "--sandbox", "read-only", "--", "--model"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--model a-model") || !strings.Contains(joined, "first prompt") {
		t.Fatalf("the alias lost something behind its terminator: %s", joined)
	}
	if !strings.HasSuffix(joined, "--sandbox read-only -- --model") {
		t.Fatalf("the typed tail was rewritten: %s", joined)
	}
}

// The joined short form in the same company: a terminator in the alias stops
// it from replacing anything, whichever way it is written.
func TestAJoinedShortFormAfterATerminatorIsInput(t *testing.T) {
	s := set(t, `
[alias.x]
harness = "codex"
args = ["-ma-model", "--"]
`)
	got, err := s.Expand([]string{"x", "-mtyped"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "-ma-model") {
		t.Fatalf("a word of the prompt replaced the alias setting: %s", joined)
	}
	if !strings.HasSuffix(joined, "-- -mtyped") {
		t.Fatalf("the typed word did not stay input: %s", joined)
	}
}

// -mvalue is the same parameter as --model value. Codex accepts the joined
// form and refuses the duplicate, so missing it breaks the launch.
func TestAJoinedShortFormOnTheLineReplaces(t *testing.T) {
	s := set(t, wcodex)
	got, err := s.Expand([]string{"wcodex", "-mtyped"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "a-model") {
		t.Fatalf("the alias model survived a joined short form: %s", joined)
	}
	if !strings.Contains(joined, "-mtyped") {
		t.Fatalf("the typed model is missing: %s", joined)
	}
}

// And the other way around: the alias writes it joined, the line writes it long.
func TestAJoinedShortFormInTheAliasIsReplaced(t *testing.T) {
	s := set(t, `
[alias.x]
harness = "codex"
args = ["-ma-model", "--sandbox", "read-only"]
`)
	got, err := s.Expand([]string{"x", "--model", "typed"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "-ma-model") {
		t.Fatalf("the joined form in the alias survived: %s", joined)
	}
	if !strings.Contains(joined, "--sandbox read-only") {
		t.Fatalf("an unrelated flag was dropped: %s", joined)
	}
}

// And before the alias name: a terminator ends rewake's own flags too, so the
// word behind it is not a command and not an alias.
func TestATerminatorBeforeTheNameLeavesItAlone(t *testing.T) {
	s := set(t, wcodex)
	got, err := s.Expand([]string{"--", "wcodex"}, []string{"codex"}, valued, nil, codexLike)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, " ") != "-- wcodex" {
		t.Fatalf("an alias behind the terminator was expanded: %v", got)
	}
}
