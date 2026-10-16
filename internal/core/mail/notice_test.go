package mail_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func TestNoticesPreviewOnlyTheAuthorsFirstLine(t *testing.T) {
	for kind, tail := range map[mail.Kind]string{mail.Task: previewOnly, mail.Finished: "", mail.Error: ""} {
		message := mail.Message{From: "writer", Kind: kind, Text: "The result is ready\nfull details stay in inbox"}
		got := mail.Notice(message)
		if !strings.HasSuffix(got, "\n  ↳ The result is ready"+tail) || strings.Contains(got, "full details") {
			t.Errorf("notice=%q", got)
		}
	}
	if got := mail.Notice(mail.Message{Text: "\nnot a preview"}); strings.Contains(got, "\n") {
		t.Errorf("empty first line was skipped: %q", got)
	}
}

func TestPreviewsAreBoundedAndCannotAddTerminalLines(t *testing.T) {
	for _, line := range []string{strings.Repeat("x", 200), strings.Repeat("界", 200)} {
		got := mail.Notice(mail.Message{From: "writer", Kind: mail.Note, Text: line + "\nprivate tail"})
		parts := strings.Split(got, "\n")
		if len(parts) != 2 || utf8.RuneCountInString(parts[1]) > 100 || !strings.HasSuffix(parts[1], "…") {
			t.Errorf("unbounded preview=%q", got)
		}
	}
	got := mail.Notice(mail.Message{Kind: mail.Note, Text: "one\rsecond\nthird"})
	if !strings.HasSuffix(got, "\n  ↳ one") {
		t.Errorf("extra lines leaked: %q", got)
	}
}

func TestWidePreviewFitsOneHundredColumns(t *testing.T) {
	for _, count := range []int{48, 49, 50, 200} {
		parts := strings.Split(mail.Notice(mail.Message{Kind: mail.Note, Text: strings.Repeat("界", count)}), "\n")
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

// previewOnly is how the preview of a letter that asks for work ends.
const previewOnly = " (a preview only: run rewake inbox)"

// A task's or a question's preview says it is only a preview, alone or as the
// latest of a batch, so an agent takes the letter rather than acting on the
// line; a note's and a report's do not.
func TestATaskOrQuestionPreviewSaysItIsOnlyAPreview(t *testing.T) {
	for kind, tail := range map[mail.Kind]string{mail.Task: previewOnly, mail.Question: previewOnly, "": previewOnly, mail.Note: "", mail.Finished: ""} {
		message := mail.Message{ID: "b", From: "lead", Kind: kind, Text: "read word.txt and reply with it\nthe rest"}
		if got, want := mail.Notice(message), "\n  ↳ read word.txt and reply with it"+tail; !strings.HasSuffix(got, want) {
			t.Errorf("%q: notice = %q, want it to end %q", kind, got, want)
		}
		older := mail.Message{ID: "a", From: "peer", Kind: mail.Note, Text: "build finished"}
		group := mail.Message{ID: "group", Batch: []mail.Message{older, message}}
		want := "Rewake: 2 new messages\n  ↳ lead " + string(mail.KindOf(message)) + ": read word.txt and reply with it" + tail
		if got := mail.Notice(group); got != want {
			t.Errorf("%q: grouped notice = %q, want %q", kind, got, want)
		}
	}
	older := mail.Message{ID: "a", From: "lead", Kind: mail.Task, Text: "do the work"}
	latest := mail.Message{ID: "b", From: "peer", Kind: mail.Note, Text: "build finished"}
	if got := mail.Notice(mail.Message{ID: "group", Batch: []mail.Message{older, latest}}); strings.Contains(got, previewOnly) {
		t.Errorf("a batch whose latest is a note: %q", got)
	}
}

// The bound cuts the author's text and never the words that say it is a
// preview: the line stays within 100 columns and ends with them.
func TestThePreviewOnlyWordsAreNeverCut(t *testing.T) {
	room := 96 - len(previewOnly)
	for _, text := range []string{strings.Repeat("x", room), strings.Repeat("x", room+1), strings.Repeat("界", 200)} {
		for _, message := range []mail.Message{
			{ID: "b", From: "lead", Kind: mail.Question, Text: text},
			{ID: "group", Batch: []mail.Message{{ID: "a", Kind: mail.Note, Text: "n"}, {ID: "b", From: "lead", Kind: mail.Task, Text: text}}},
		} {
			parts := strings.Split(mail.Notice(message), "\n")
			line := parts[len(parts)-1]
			// These glyphs have known widths; do not use the production estimator.
			cells := len([]rune(line)) + strings.Count(line, "界")
			if cells > 100 || !strings.HasSuffix(line, previewOnly) {
				t.Fatalf("line of %d columns: %q", cells, line)
			}
			if fits := len(message.Batch) == 0 && text == strings.Repeat("x", room); fits == strings.Contains(line, "…") {
				t.Fatalf("cut where it fits, or not where it does not: %q", line)
			}
		}
	}
}
