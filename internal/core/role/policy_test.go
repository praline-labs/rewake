package role

import (
	"strings"
	"testing"
)

// The delegating main is the catalog's own playbook, unchanged: a launch
// that carries no policy — a worker, a guide outside a session, an old record
// — reads exactly what it read before main could opt in.
func TestTheDefaultMainDelegatesExactlyAsBefore(t *testing.T) {
	if got, want := For(Main, false), Main.Play; !samePlaybook(got, want) {
		t.Fatal("the default main playbook changed")
	}
	for _, part := range []Role{Write, General} {
		if got := For(part, true); !samePlaybook(got, part.Play) {
			t.Errorf("%s took main's policy", part.ID)
		}
	}
}

// An opted-in main may implement, and its playbook says so in the one place
// the policy changes the craft; nothing else moves.
func TestAnOptingInMainGetsTheImplementingRule(t *testing.T) {
	delegating, implementing := Main.Play, For(Main, true)
	if samePlaybook(delegating, implementing) {
		t.Fatal("an opted-in main got the delegating playbook")
	}
	for index, section := range implementing.Sections {
		before := delegating.Sections[index]
		if section.Title != before.Title {
			t.Fatalf("section %d became %q", index, section.Title)
		}
		if section.Title != handingOutWork {
			if strings.Join(section.Lines, "\n") != strings.Join(before.Lines, "\n") {
				t.Errorf("%s changed with the policy", section.Title)
			}
			continue
		}
		if section.Lines[0] != implementingRule {
			t.Fatalf("the first rule is %q", section.Lines[0])
		}
		if strings.Contains(strings.Join(section.Lines, " "), "Do not write product code yourself") {
			t.Error("the delegation prohibition survived the opt-in")
		}
		if !strings.Contains(section.Lines[0], "grants no additional permissions") {
			t.Error("the implementing rule claims a permission")
		}
		if !sameLines(section.Lines[1:], before.Lines[1:]) {
			t.Error("the rest of the section changed with the policy")
		}
	}
	// The catalog itself is untouched: the next launch of a project that
	// did not opt in gets the prohibition, not this project's policy.
	if Main.Play.Sections[0].Lines[0] != delegatingRule || strings.Contains(Main.Play.Sections[0].Lines[0], "may implement") {
		t.Fatal("For mutated the singleton main playbook")
	}
}

// General's Git rule names its own prohibition, not a claim about who may
// commit: an authorized implementing main is no longer excluded by it.
func TestGeneralsGitLineNamesItsOwnProhibition(t *testing.T) {
	var git string
	for _, section := range General.Play.Sections {
		if section.Title != "MAIN AND PERMISSIONS" {
			continue
		}
		git = section.Lines[len(section.Lines)-1]
	}
	if git != "Do not write .git or commit; report your work to main." {
		t.Fatalf("general's Git line is %q", git)
	}
	if Write.Play.Sections[2].Lines[2] != "You may commit when authorized, within your own permissions." {
		t.Fatal("write's Git line changed with general's")
	}
}

func samePlaybook(a, b Playbook) bool {
	if a.Heading != b.Heading || len(a.Steps) != len(b.Steps) || len(a.Sections) != len(b.Sections) {
		return false
	}
	for index := range a.Steps {
		if a.Steps[index] != b.Steps[index] {
			return false
		}
	}
	for index := range a.Sections {
		if a.Sections[index].Title != b.Sections[index].Title || !sameLines(a.Sections[index].Lines, b.Sections[index].Lines) {
			return false
		}
	}
	return true
}

func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index] != b[index] {
			return false
		}
	}
	return true
}
