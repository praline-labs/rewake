//go:build rewakefixture

package fixture

import (
	"time"

	"github.com/praline-labs/rewake/internal/sessionstate"
)

// compactions is what the adapter keeps of the program's compactions: how many
// ended, the cue of each, and the outcome of each one a main asked for, which
// that main's wrapper sends as a letter. Kept across reconnections — they are
// the program's — and published only while Control and Telemetry are live.
type compactions struct {
	count    uint64
	events   []sessionstate.CompactionEvent
	outcomes []sessionstate.CompactionOutcome
}

// maxCompactionEvents bounds the cues a snapshot carries, as Claude Code's
// collector bounded its own.
const maxCompactionEvents = 64

// compacted takes the end of one compaction while Control is live on the
// connection it came on: a done one is counted, and one a main asked for
// leaves its outcome. A refusal or a failure counts nothing.
func (b *backend) compacted(l *link, frame Frame) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if l == nil || b.link != l || !b.live[Control] {
		return
	}
	now := time.Now()
	if frame.Outcome == "done" {
		b.compact.count++
		b.compact.events = append(b.compact.events, sessionstate.CompactionEvent{
			Sequence: b.compact.count, ObservedAt: now, RequestedBy: frame.From, Request: frame.Request,
		})
		if len(b.compact.events) > maxCompactionEvents {
			b.compact.events = b.compact.events[len(b.compact.events)-maxCompactionEvents:]
		}
	}
	if frame.Request != "" && frame.From != "" {
		b.compact.outcomes = sessionstate.KeepOutcome(b.compact.outcomes, sessionstate.CompactionOutcome{
			Request: frame.Request, RequestedBy: frame.From, Outcome: frame.Outcome, Reason: frame.Reason,
			Detail: frame.Detail, TokensBefore: frame.TokensBefore, TokensAfter: frame.TokensAfter, EndedAt: now,
		})
	}
}

// publishCompactions adds the compactions to a snapshot taken under b.mu
// while Control is live: the count is known only from a program that can
// compact on request.
func (b *backend) publishCompactions(snapshot *sessionstate.Snapshot) {
	if !b.live[Control] {
		return
	}
	count := b.compact.count
	snapshot.Compactions, snapshot.Coverage = &count, "observed"
	snapshot.CompactionEvents = append([]sessionstate.CompactionEvent{}, b.compact.events...)
	snapshot.CompactionOutcomes = append([]sessionstate.CompactionOutcome{}, b.compact.outcomes...)
}
