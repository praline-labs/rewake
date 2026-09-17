package claude

import (
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func TestFailureNoticesUseFailedStatus(t *testing.T) {
	got := notification(inbox.Message{ID: "id", From: "review", Kind: inbox.Error})
	if !strings.Contains(got, "<status>failed</status>") || !strings.Contains(got, "review error") {
		t.Fatalf("notice=%s", got)
	}
}
