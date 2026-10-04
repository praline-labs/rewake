package channel_test

import (
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
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
		letter := inbox.Message{ID: "1", From: sender, Kind: inbox.Note, Text: first + "\nsecond line", CreatedAt: time.Now()}
		other := inbox.Message{ID: "0", From: "other", Kind: inbox.Note, Text: "x", CreatedAt: time.Now().Add(-time.Second)}
		alone := harness.Notice(letter)
		grouped := harness.Notice(inbox.Message{Batch: []inbox.Message{other, letter}})
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
