package mail_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func TestNoticesPreviewOnlyTheAuthorsFirstLine(t *testing.T) {
	for _, kind := range []mail.Kind{mail.Task, mail.Finished, mail.Error} {
		message := mail.Message{From: "writer", Kind: kind, Text: "The result is ready\nfull details stay in inbox"}
		got := mail.Notice(message)
		if !strings.HasSuffix(got, "\n  ↳ The result is ready") || strings.Contains(got, "full details") {
			t.Errorf("notice=%q", got)
		}
	}
	if got := mail.Notice(mail.Message{Text: "\nnot a preview"}); strings.Contains(got, "\n") {
		t.Errorf("empty first line was skipped: %q", got)
	}
}

func TestPreviewsAreBoundedAndCannotAddTerminalLines(t *testing.T) {
	for _, line := range []string{strings.Repeat("x", 200), strings.Repeat("界", 200)} {
		got := mail.Notice(mail.Message{From: "writer", Text: line + "\nprivate tail"})
		parts := strings.Split(got, "\n")
		if len(parts) != 2 || utf8.RuneCountInString(parts[1]) > 100 || !strings.HasSuffix(parts[1], "…") {
			t.Errorf("unbounded preview=%q", got)
		}
	}
	got := mail.Notice(mail.Message{Text: "one\rsecond\nthird"})
	if !strings.HasSuffix(got, "\n  ↳ one") {
		t.Errorf("extra lines leaked: %q", got)
	}
}

func TestWidePreviewFitsOneHundredColumns(t *testing.T) {
	for _, count := range []int{48, 49, 50, 200} {
		parts := strings.Split(mail.Notice(mail.Message{Text: strings.Repeat("界", count)}), "\n")
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

func TestGroupedNoticeKeepsCompactPreviewWithoutInstructions(t *testing.T) {
	latest := mail.Message{ID: "b", From: "writer", Kind: mail.Finished, Text: "Result ready\nprivate details"}
	old := mail.Message{ID: "a", From: "sender", Kind: mail.Task, Text: "older summary"}
	message := mail.Message{ID: "group", Batch: []mail.Message{latest, old}}
	want := "Rewake: 2 new messages\n  ↳ writer finished: Result ready"
	if got := mail.Notice(message); got != want {
		t.Fatalf("notice = %q, want %q", got, want)
	}
	for _, body := range []string{strings.Repeat("界", 200) + "\nprivate details", "\nprivate details", "one\rhidden\nprivate details", "one\u2028hidden"} {
		message.Batch[0].Text = body
		got := mail.Notice(message)
		parts := strings.Split(got, "\n")
		if len(parts) != 2 || !strings.HasPrefix(parts[1], "  ↳ writer finished: ") || strings.Contains(got, "private details") || strings.Contains(got, "hidden") || strings.Contains(got, "Run rewake") || strings.Contains(got, "--peek") {
			t.Fatal(got)
		}
		width := 0
		for _, r := range parts[1] {
			if r == '界' {
				width += 2
			} else {
				width++
			}
		}
		if width > 100 {
			t.Fatalf("preview width %d", width)
		}
	}
}
