//go:build rewaketest

package fixture

import (
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
)

// compacted takes the end of one compaction while Control is live on the
// connection it came on: a done one is counted, and one a main asked for
// leaves its outcome. A refusal or a failure counts nothing.
func (b *backend) compacted(l *link, frame Frame) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if l == nil || b.link != l || !b.live[Control] {
		return
	}
	// The program numbers no compaction, and its count is kept across
	// reconnections: one source, each end counted as it comes.
	if frame.Outcome == "done" {
		b.observed.Compaction(compactionSource, 0, frame.From, frame.Request)
	}
	b.observed.Outcome(session.CompactionOutcome{
		Request: frame.Request, RequestedBy: frame.From, Outcome: frame.Outcome, Reason: frame.Reason,
		Detail: frame.Detail, TokensBefore: frame.TokensBefore, TokensAfter: frame.TokensAfter, EndedAt: time.Now(),
	})
}

// compactionSource is the one source the program's compactions are counted
// from.
const compactionSource = "program"
