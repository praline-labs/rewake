package alias

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// valuedWithCommand adds rewake's --command to the flags that take a value.
func valuedWithCommand(name string) bool { return valued(name) || name == CommandFlag }

// userSet reads contents as the user's own alias file.
func userSet(t *testing.T, contents string) *Set {
	t.Helper()
	path := filepath.Join(t.TempDir(), userFile)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newSet()
	s.read(path, "the user alias file", true)
	return s
}

const review = `
[alias.review]
harness = "claude"
rewake  = ["--general", "--name", "review"]
args    = []
command = "my-claude"
`

// The command field becomes rewake's --command, before the harness word, so
// the whole launch is one name.
func TestTheCommandFieldBecomesTheFlag(t *testing.T) {
	got, err := userSet(t, review).Expand([]string{"review"}, []string{"claude"}, valuedWithCommand, []string{"general", CommandFlag}, agentLike)
	if err != nil {
		t.Fatal(err)
	}
	want := "--command my-claude --general --name review claude"
	if strings.Join(got, " ") != want {
		t.Errorf("got %q, want %q", strings.Join(got, " "), want)
	}
}

// A --command typed on the line replaces the alias's, as rewake's own flags do.
func TestATypedCommandReplacesTheAliasCommand(t *testing.T) {
	for _, typed := range [][]string{{"--command", "other"}, {"--command=other"}} {
		argv := append(append([]string{}, typed...), "review")
		got, err := userSet(t, review).Expand(argv, []string{"claude"}, valuedWithCommand, nil, agentLike)
		if err != nil {
			t.Fatal(err)
		}
		if joined := strings.Join(got, " "); strings.Contains(joined, "my-claude") || strings.Count(joined, "--command") != 1 {
			t.Errorf("%v: got %q, want only the typed command", typed, joined)
		}
	}
}

// A file carried by a directory may not choose the program a launch starts,
// in either spelling.
func TestAProjectAliasMayNotChooseTheProgram(t *testing.T) {
	for name, contents := range map[string]string{
		"field":  "[alias.x]\nharness = \"claude\"\ncommand = \"my-claude\"\n",
		"flag":   "[alias.x]\nharness = \"claude\"\nrewake = [\"--command\", \"my-claude\"]\n",
		"joined": "[alias.x]\nharness = \"claude\"\nrewake = [\"--command=my-claude\"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := set(t, contents).Expand([]string{"x"}, []string{"claude"}, valuedWithCommand, []string{"main", CommandFlag}, agentLike)
			if err == nil || !strings.Contains(err.Error(), "the program a launch starts belongs in the user alias file") {
				t.Fatalf("got %v, want the program refused in a project file", err)
			}
		})
	}
}

// The program named twice in one alias is refused rather than guessed.
func TestAnAliasNamingTheProgramTwiceIsRefused(t *testing.T) {
	s := userSet(t, "[alias.x]\nharness = \"claude\"\ncommand = \"a\"\nrewake = [\"--command\", \"b\"]\n")
	if _, err := s.Expand([]string{"x"}, []string{"claude"}, valuedWithCommand, nil, agentLike); err == nil || !strings.Contains(err.Error(), "names the program twice") {
		t.Fatalf("got %v", err)
	}
}
