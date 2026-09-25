package sessionstate

import (
	"strings"
	"testing"
	"time"
)

// A snapshot at its worst — every compaction event a main's, with the longest
// names, and outcomes whose details are the longest the host or the server
// could send, in characters JSON escapes — still saves: an outcome history
// over the snapshot's limit would stop the whole telemetry. The newest
// outcome is always kept, and each detail is cut.
func TestAWorstCaseSnapshotStillSaves(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	name := strings.Repeat("m", 32)
	request := strings.Repeat("f", 32)
	text := strings.Repeat("x", 128)
	used, window, percent, count, compacting := int64(1<<40), int64(1<<40), 100, uint64(1<<40), true
	snapshot := Snapshot{
		Activity: &text, WaitingFor: []string{"approval", "input"}, ActivityAt: &now, ActivityFresh: true,
		Thread: strings.Repeat("t", 64), Selection: "ready", PublishedAt: &now, ObservedAt: &now, Fresh: true, ContextFresh: true, SettingsFresh: true,
		ContextUsed: &used, ContextWindow: &window, FilledPercent: &percent, ContextAt: &now, Model: &text, Effort: &text, SettingsAt: &now,
		Compactions: &count, Compacting: &compacting, Coverage: "observed", Interruptions: InterruptionsUnobserved,
	}
	for i := range 64 {
		snapshot.CompactionEvents = append(snapshot.CompactionEvents, CompactionEvent{Sequence: count - uint64(i), ObservedAt: now, RequestedBy: name, Request: request})
	}
	var outcomes []CompactionOutcome
	for i := range 20 {
		tokens := int64(1 << 40)
		outcomes = KeepOutcome(outcomes, CompactionOutcome{
			Request: request, RequestedBy: name, Outcome: "failed", Reason: "another request in flight",
			Detail: strings.Repeat("<\"\x01é", 256) + string(rune('a'+i)), TokensBefore: &tokens, TokensAfter: &tokens, EndedAt: now,
		})
	}
	snapshot.CompactionOutcomes = outcomes
	if err := Save(dir, name, "1.2", snapshot); err != nil {
		t.Fatalf("the worst case does not save: %v", err)
	}
	if newest := outcomes[len(outcomes)-1]; len(newest.Detail) > maxOutcomeDetail+len("…") || !strings.HasPrefix(newest.Detail, "<\"\x01é") {
		t.Fatalf("the newest outcome's detail: %d bytes %q", len(newest.Detail), newest.Detail[:16])
	}
	if got := Load(dir, name, "1.2"); len(got.CompactionOutcomes) != len(outcomes) {
		t.Fatalf("read back %d outcomes of %d", len(got.CompactionOutcomes), len(outcomes))
	}
}
