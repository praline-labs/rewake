package codex

import (
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
)

func TestFailureNoticesAreRed(t *testing.T) {
	if noticePrefix(inbox.Message{Kind: inbox.Error}) != "🔴" {
		t.Fatal("error is not red")
	}
}

func TestInterimNoticesHaveTheirOwnMark(t *testing.T) {
	if noticePrefix(inbox.Message{Kind: inbox.Interim}) != "⏳" {
		t.Error("an interim report looks like another outcome")
	}
}
