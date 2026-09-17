package codex

import (
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func TestFailureNoticesAreRed(t *testing.T) {
	if noticePrefix(inbox.Message{Kind: inbox.Error}) != "🔴" {
		t.Fatal("error is not red")
	}
}
