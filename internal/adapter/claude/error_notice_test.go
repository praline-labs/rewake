package claude

import (
	"strings"
	"testing"

	mail "github.com/praline-labs/rewake/internal/inbox"
)

func TestFailureNoticesUseFailedStatus(t *testing.T) {
	got := notification(mail.Message{ID: "id", From: "review", Kind: mail.Error})
	if !strings.Contains(got, "<status>failed</status>") || !strings.Contains(got, "review error") {
		t.Fatalf("notice=%s", got)
	}
}

// An interim report is neither completed nor failed: it says the work is still
// going, in a status the interface draws without either color.
func TestInterimNoticesAreNeitherDoneNorFailed(t *testing.T) {
	notice := notification(mail.Message{ID: "1790000000000000000-abcdef012345", From: "write-claude", Kind: mail.Interim, Text: "the suite is running", Unread: 1})
	if !strings.Contains(notice, "<status>running</status>") || !strings.Contains(notice, "write-claude pending") {
		t.Errorf("notice = %s", notice)
	}
}
