package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

// The module's word of how a compaction a main asked for ended decodes with
// its outcome and counts, a detail too long for a datagram is cut, and an
// outcome a compaction cannot have is not taken.
func TestACompactionEndedDecodes(t *testing.T) {
	got, ok := DecodePlugin([]byte(`{"plugin_event":"compact.ended","request":"` + askedID + `","by":"lead","outcome":"done","tokensBefore":120000,"tokensAfter":9000}`))
	if !ok || got.Kind != CompactEnded || got.Request != askedID || got.By != "lead" || got.Outcome != control.Done ||
		got.TokensBefore == nil || *got.TokensBefore != 120000 || got.TokensAfter == nil || *got.TokensAfter != 9000 {
		t.Fatalf("done: %+v, %v", got, ok)
	}
	long := strings.Repeat("é", 300)
	got, ok = DecodePlugin([]byte(`{"plugin_event":"compact.ended","request":"` + askedID + `","by":"lead","outcome":"failed","detail":"` + long + `"}`))
	if !ok || got.Outcome != control.Failed || len(got.Detail) > maxDetail+len("…") || !strings.HasSuffix(got.Detail, "…") || !strings.HasPrefix(long, strings.TrimSuffix(got.Detail, "…")) {
		t.Fatalf("a long detail: %+v, %v", got, ok)
	}
	for _, raw := range []string{
		`{"plugin_event":"compact.ended","by":"lead","outcome":"done"}`,
		`{"plugin_event":"compact.ended","request":"` + askedID + `","outcome":"done"}`,
		`{"plugin_event":"compact.ended","request":"` + askedID + `","by":"lead","outcome":"started"}`,
		`{"plugin_event":"compact.ended","request":"` + askedID + `","by":"lead"}`,
	} {
		if event, ok := DecodePlugin([]byte(raw)); ok {
			t.Errorf("DecodePlugin(%s) = %+v, want refused", raw, event)
		}
	}
}

// The outcome is kept in the snapshot, by request and asker, for main's
// wrapper to send as a letter.
func TestACompactionEndedIsKept(t *testing.T) {
	before, after := int64(120000), int64(9000)
	var f folding
	for _, event := range []Event{
		{Kind: SessionStart},
		{Kind: CompactAsked, Request: askedID, By: "lead"},
		{Kind: PreCompact},
		{Kind: PostCompact},
		{Kind: CompactEnded, Request: askedID, By: "lead", Outcome: control.Done, TokensBefore: &before, TokensAfter: &after},
	} {
		f.send(event)
	}
	got := f.state.snapshot().CompactionOutcomes
	if len(got) != 1 || got[0].Request != askedID || got[0].RequestedBy != "lead" || got[0].Outcome != control.Done ||
		*got[0].TokensBefore != before || *got[0].TokensAfter != after || got[0].EndedAt.IsZero() {
		t.Fatalf("outcomes %+v", got)
	}
}

// The PreCompact of the compaction a main asked for leaves the mark the
// module watches for to answer started; a PreCompact nobody asked for, or one
// without a control directory, leaves none.
func TestTheCollectorMarksTheStartOfAnAskedCompaction(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	controls := t.TempDir()
	collector := NewCollector(path)
	collector.Controls(controls)
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	Send(path, Event{Kind: SessionStart})
	Send(path, Event{Kind: PreCompact})
	Send(path, Event{Kind: PostCompact})
	Send(path, Event{Kind: CompactAsked, Request: askedID, By: "lead"})
	Send(path, Event{Kind: PreCompact})
	marked := control.StartedPath(controls, askedID)
	waitFor(t, "the mark", func() bool { _, err := os.Stat(marked); return err == nil })
	time.Sleep(20 * time.Millisecond)
	entries, _ := os.ReadDir(controls)
	if len(entries) != 1 {
		t.Fatalf("control directory holds %v", entries)
	}
}
