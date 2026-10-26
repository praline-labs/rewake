package taskbrief

import (
	"strings"
	"testing"
)

const sample = "# Auth refactor\n" +
	"\n" +
	"Move the login to the new service.\n" +
	"\n" +
	"- [ ] write the client\n" +
	"  * [x] nested, done by main\n" +
	"1. [ ] numbered\n" +
	"2) [X] numbered too\n" +
	"- [] not a box\n" +
	"- [ ]no blank after the box\n" +
	"```\n" +
	"- [ ] inside a fence\n" +
	"``` info\n" +
	"- [ ] still inside: a closing fence takes no info\n" +
	"```\n" +
	"~~~ go\n" +
	"- [ ] inside a tilde fence\n" +
	"~~~\n" +
	"+ [ ] after the fences\r\n"

func TestTheScanFindsTaskLinesOutsideFences(t *testing.T) {
	items := scan(sample)
	want := []struct {
		text string
		done bool
	}{
		{"write the client", false},
		{"nested, done by main", true},
		{"numbered", false},
		{"numbered too", true},
		{"after the fences", false},
	}
	if len(items) != len(want) {
		t.Fatalf("items = %+v, want %d", items, len(want))
	}
	for index, item := range items {
		if item.N != index+1 || item.Text != want[index].text || item.Done != want[index].done {
			t.Errorf("item %d = %+v, want %+v", index+1, item, want[index])
		}
	}
}

func TestTheTitleIsTheFirstHeadingElseTheFirstLine(t *testing.T) {
	cases := map[string]string{
		sample:                                 "Auth refactor",
		"\n\nno heading here\n# later":         "later",
		"```\n# in a fence\n```\nplain first?": "```",
		"  first line\n## second level":        "first line",
		"# " + strings.Repeat("é", 70):         strings.Repeat("é", 60),
	}
	for text, want := range cases {
		if got := title(text); got != want {
			t.Errorf("title(%q) = %q, want %q", text, got, want)
		}
	}
}

// The numbered form puts the number after the box, and the ticks change only
// the box's character: the line keeps the rest as main wrote it.
func TestRenderSetsTheBoxesAndNumbers(t *testing.T) {
	text := "- [ ] one\n  * [X] two\n1. [x] three\r\n"
	items := scan(text)
	items[0].Done, items[1].Done = true, true
	items[2].Done = false
	if got, want := render(text, items, false), "- [x] one\n  * [X] two\n1. [ ] three\r\n"; got != want {
		t.Errorf("markdown = %q, want %q", got, want)
	}
	if got, want := render(text, items, true), "- [x] #1 one\n  * [X] #2 two\n1. [ ] #3 three\r\n"; got != want {
		t.Errorf("numbered = %q, want %q", got, want)
	}
	if again := scan(render(text, items, false)); again[0].Done != true || again[2].Done != false {
		t.Errorf("the markdown reads back as %+v", again)
	}
}
