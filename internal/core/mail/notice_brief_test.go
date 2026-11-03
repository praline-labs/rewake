package mail_test

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A report on a brief's task ends its preview with the brief's note, which
// the bound on the line never cuts; any other kind shows no note.
func TestAReportsPreviewEndsWithTheBriefsNote(t *testing.T) {
	short := mail.Notice(mail.Message{From: "writer", Kind: mail.Finished, Text: "login moved"}.WithBriefNote("brief 96 7/8"))
	if !strings.HasSuffix(short, "\n  ↳ login moved (brief 96 7/8)") {
		t.Errorf("notice=%q", short)
	}
	long := mail.Notice(mail.Message{From: "writer", Kind: mail.Finished, Text: strings.Repeat("x", 200)}.WithBriefNote("brief 96 7/8"))
	preview := strings.Split(long, "\n")[1]
	if len([]rune(preview)) > 100 || !strings.HasSuffix(preview, "…"+" (brief 96 7/8)") {
		t.Errorf("long preview=%q", preview)
	}
	if got := mail.Notice(mail.Message{From: "writer", Kind: mail.Note, Text: "hello"}.WithBriefNote("brief 96 7/8")); strings.Contains(got, "brief") {
		t.Errorf("a note shows the brief: %q", got)
	}
}

// A note too long for the line is left off rather than cutting the author's text.
func TestAnOverlongBriefNoteIsLeftOff(t *testing.T) {
	got := mail.Notice(mail.Message{From: "writer", Kind: mail.Finished, Text: "login moved"}.WithBriefNote(strings.Repeat("brief 9 1/2, ", 8)))
	if !strings.HasSuffix(got, "\n  ↳ login moved") {
		t.Errorf("notice=%q", got)
	}
}
