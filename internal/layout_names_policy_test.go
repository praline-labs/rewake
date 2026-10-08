package internal

import (
	"fmt"
	"slices"
	"testing"
)

// A catalog with the test-only harness in it, as a tagged build has.
var taggedCatalog = [][2]string{{"claude", "Claude Code"}, {"relay", "Relay"}, {"fixture", "Fixture harness (tests only)"}}

const policySource = `package mail

// the fixture reads its mail, and claude would not
var fixtureTimes = 1

var a = "the fixture frame"

var b = "fixture"

var c = "relay"

var d = "Fixture harness (tests only)"
`

func policyMentions(t *testing.T, testOnly []string) []string {
	t.Helper()
	words, literals := nameWords(taggedCatalog, testOnly)
	found, err := findMentions("internal/core/mail/mail.go", []byte(policySource), words, literals...)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range found {
		got = append(got, fmt.Sprintf("%d %s %q", m.line, m.kind, m.word))
	}
	slices.Sort(got)
	return got
}

// The test-only harness is found by its title anywhere and by its id as a
// whole string literal; its id in prose, an identifier or a longer string is
// the ordinary noun. The shipped harnesses keep the whole rule beside it.
func TestATestOnlyHarnessIsFoundByItsTitleAndItsIDAsALiteral(t *testing.T) {
	want := []string{`10 string "relay"`, `12 string "fixture harness (tests only)"`, `3 comment "claude"`, `8 string "fixture"`}
	if got := policyMentions(t, []string{"fixture"}); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// The same harness shipped is looked for by its id in every text, as any
// harness the product carries: the policy is for test-only harnesses alone.
func TestAShippedHarnessIsFoundByItsIDInEveryText(t *testing.T) {
	want := []string{
		`10 string "relay"`, `12 string "fixture"`, `3 comment "claude"`, `3 comment "fixture"`,
		`4 identifier "fixture"`, `6 string "fixture"`, `8 string "fixture"`,
	}
	if got := policyMentions(t, nil); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Under the policy the shipped ids stay words, and the test-only id is a
// literal and not a word.
func TestTheTestOnlyIDIsALiteralAndNotAWord(t *testing.T) {
	words, literals := nameWords(taggedCatalog, []string{"fixture"})
	if slices.Contains(words, "fixture") || !slices.Equal(literals, []string{"fixture"}) ||
		!slices.Contains(words, "claude") || !slices.Contains(words, "relay") {
		t.Fatalf("words %q, literals %q", words, literals)
	}
}
