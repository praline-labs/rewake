package codex

import (
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

const noticeMark = "🟢"

func noticePrefix(message inbox.Message) string {
	switch harness.NoticeKind(message) {
	case inbox.Error:
		return "🔴"
	case inbox.Stopped:
		return "🟡"
	default:
		return noticeMark
	}
}
