package reportbrief

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A report answering eight briefs names two and counts the rest, so its
// preview keeps the author's text first.
func TestAReportOnEightBriefsNamesTwoAndCountsTheRest(t *testing.T) {
	var blocks []Block
	for id := 90; id < 98; id++ {
		blocks = append(blocks, Block{ID: id, Done: 3, Total: 8})
	}
	note := Notes(blocks)
	if note != "brief 90 3/8, brief 91 3/8, +6" {
		t.Fatalf("note = %q", note)
	}
	text := "Part A verdict: the migration is merged and every check is green on the new schema"
	preview := strings.Split(mail.Notice(mail.Message{From: "writer", Kind: mail.Finished, Text: text}.WithBriefNote(note)), "\n")[1]
	if len([]rune(preview)) > 100 || !strings.Contains(preview, "Part A verdict: the migration is merged") || !strings.HasSuffix(preview, "(brief 90 3/8, brief 91 3/8, +6)") {
		t.Fatalf("preview = %q", preview)
	}
}
