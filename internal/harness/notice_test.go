package harness

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func TestNoticesPreviewOnlyTheAuthorsFirstLine(t *testing.T) {
	for _, kind := range []inbox.Kind{inbox.Task, inbox.Finished, inbox.Error} {
		message := inbox.Message{From: "writer", Kind: kind, Text: "The result is ready\nfull details stay in inbox"}
		got := Notice(message)
		if !strings.HasSuffix(got, "\n  ↳ The result is ready") || strings.Contains(got, "full details") {
			t.Errorf("notice=%q", got)
		}
	}
	if got := Notice(inbox.Message{Text: "\nnot a preview"}); strings.Contains(got, "\n") {
		t.Errorf("empty first line was skipped: %q", got)
	}
}

func TestPreviewsAreBoundedAndCannotAddTerminalLines(t *testing.T) {
	for _, line := range []string{strings.Repeat("x", 200), strings.Repeat("界", 200)} {
		got := Notice(inbox.Message{From: "writer", Text: line + "\nprivate tail"})
		parts := strings.Split(got, "\n")
		if len(parts) != 2 || utf8.RuneCountInString(parts[1]) > 100 || !strings.HasSuffix(parts[1], "…") {
			t.Errorf("unbounded preview=%q", got)
		}
	}
	got := Notice(inbox.Message{Text: "one\rsecond\nthird"})
	if !strings.HasSuffix(got, "\n  ↳ one") {
		t.Errorf("extra lines leaked: %q", got)
	}
}

func TestWidePreviewFitsOneHundredColumns(t *testing.T) {
	for _, count := range []int{48, 49, 50, 200} {
		parts := strings.Split(Notice(inbox.Message{Text: strings.Repeat("界", count)}), "\n")
		if len(parts) != 2 || !strings.HasPrefix(parts[1], "  ↳ ") {
			t.Fatalf("missing preview: %q", parts)
		}
		line := parts[1]
		// These glyphs have known widths; do not use the production estimator.
		cells := 4 + 2*strings.Count(line, "界") + strings.Count(line, "…")
		if cells > 100 {
			t.Fatalf("preview occupies %d columns: %s", cells, line)
		}
		if count == 48 && strings.Contains(line, "…") {
			t.Fatal("exactly fitting preview was truncated")
		}
		if count > 48 && !strings.HasSuffix(line, "…") {
			t.Fatal("wide preview lost its truncation marker")
		}
	}
}
