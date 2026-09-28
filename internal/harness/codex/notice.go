package codex

import (
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
)

const noticeMark = "🟢"

func noticePrefix(message inbox.Message) string {
	switch harness.NoticeKind(message) {
	case inbox.Error:
		return "🔴"
	case inbox.Stopped:
		return "🟡"
	case inbox.Interim:
		return "⏳"
	default:
		return noticeMark
	}
}
