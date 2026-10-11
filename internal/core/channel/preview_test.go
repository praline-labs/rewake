package channel_test

import (
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/channel"
	"github.com/praline-labs/rewake/internal/core/mail"
)

// Every first line reaches the reader whole: alone and grouped, from a
// sender with the longest name allowed.
func TestEveryFirstLineFitsThePreview(t *testing.T) {
	sender := strings.Repeat("s", 32)
	firsts := []string{
		channel.FirstWorkerFailing, channel.FirstMainFailing, channel.FirstMainShell, channel.FirstMainNone,
		channel.FirstMainDenied, channel.FirstMainWorks, channel.FirstMainNoTool,
	}
	for _, first := range firsts {
		letter := mail.Message{ID: "1", From: sender, Kind: mail.Note, Text: first + "\nsecond line", CreatedAt: time.Now()}
		other := mail.Message{ID: "0", From: "other", Kind: mail.Note, Text: "x", CreatedAt: time.Now().Add(-time.Second)}
		alone := mail.Notice(letter)
		grouped := mail.Notice(mail.Message{Batch: []mail.Message{other, letter}})
		for _, shown := range []string{alone, grouped} {
			if !strings.Contains(shown, first) || strings.Contains(shown, "…") {
				t.Errorf("cut: %q", shown)
			}
		}
		if len([]rune(first)) > 55 {
			t.Errorf("%q is %d cells, over 55", first, len([]rune(first)))
		}
	}
}
