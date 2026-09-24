package telemetry

import (
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

const (
	askedID = "0123456789abcdef0123456789abcdef"
	otherID = "fedcba9876543210fedcba9876543210"
)

// The module's word that a main asked for a compaction decodes with the
// request and the asker, and a request id or a name that could not be one is
// not taken.
func TestACompactionAskedDecodes(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      Event
	}{
		{"asked", `{"plugin_event":"compact.asked","request":"` + askedID + `","by":"lead"}`, Event{Kind: CompactAsked, Request: askedID, By: "lead"}},
		{"refused", `{"plugin_event":"compact.refused","request":"` + askedID + `"}`, Event{Kind: CompactRefused, Request: askedID}},
		{"refused started", `{"plugin_event":"compact.refused","request":"` + askedID + `","started":true}`, Event{Kind: CompactRefused, Request: askedID, Started: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := DecodePlugin([]byte(tc.raw))
			if !ok || got.Kind != tc.want.Kind || got.Request != tc.want.Request || got.By != tc.want.By || got.Started != tc.want.Started {
				t.Errorf("DecodePlugin(%s) = %+v, %v; want %+v", tc.raw, got, ok, tc.want)
			}
		})
	}
	for _, raw := range []string{
		`{"plugin_event":"compact.asked","by":"lead"}`,
		`{"plugin_event":"compact.asked","request":"` + askedID + `"}`,
		`{"plugin_event":"compact.asked","request":"../x","by":"lead"}`,
		`{"plugin_event":"compact.asked","request":"` + askedID + `","by":"../lead"}`,
		`{"plugin_event":"compact.refused"}`,
	} {
		if event, ok := DecodePlugin([]byte(raw)); ok {
			t.Errorf("DecodePlugin(%s) = %+v, want refused", raw, event)
		}
	}
}

// compactions folds the events and answers the compactions the snapshot lists.
func compactions(events ...Event) []sessionstate.CompactionEvent {
	var f folding
	for _, event := range events {
		f.send(event)
	}
	return f.state.snapshot().CompactionEvents
}

// The compaction after a main's word is that main's; the one after it, which
// nobody asked for, is nobody's.
func TestACompactionAMainAskedForNamesIt(t *testing.T) {
	got := compactions(
		Event{Kind: SessionStart},
		Event{Kind: CompactAsked, Request: askedID, By: "lead"},
		Event{Kind: PreCompact}, Event{Kind: SessionStart, Source: "compact"}, Event{Kind: PostCompact},
		Event{Kind: PreCompact}, Event{Kind: PostCompact},
	)
	if len(got) != 2 || got[0].RequestedBy != "lead" || got[0].Request != askedID || got[0].Sequence != 1 ||
		got[1].RequestedBy != "" || got[1].Request != "" || got[1].Sequence != 2 {
		t.Fatalf("compactions %+v", got)
	}
}

// A refused request leaves no mark behind for the session's own next
// compaction; a refusal of another request does not lay this one aside.
func TestARefusedCompactionLeavesNoMark(t *testing.T) {
	got := compactions(
		Event{Kind: SessionStart},
		Event{Kind: CompactAsked, Request: askedID, By: "lead"},
		Event{Kind: CompactRefused, Request: askedID},
		Event{Kind: PreCompact}, Event{Kind: PostCompact},
	)
	if len(got) != 1 || got[0].RequestedBy != "" || got[0].Request != "" {
		t.Fatalf("after a refusal: %+v", got)
	}
	got = compactions(
		Event{Kind: SessionStart},
		Event{Kind: CompactAsked, Request: askedID, By: "lead"},
		Event{Kind: CompactRefused, Request: otherID},
		Event{Kind: PreCompact}, Event{Kind: PostCompact},
	)
	if len(got) != 1 || got[0].Request != askedID {
		t.Fatalf("after another request's refusal: %+v", got)
	}
}

// The host refuses a short conversation after PreCompact and runs no
// PostCompact; the refusal ends the compaction, so the listing does not read
// compacting until the next turn — also when the PreCompact, a background
// hook, arrives after the refusal. A refusal before the host started leaves a
// compaction in progress, someone else's, as it is.
func TestARefusalAfterPreCompactEndsTheCompaction(t *testing.T) {
	for _, tc := range []struct {
		name       string
		events     []Event
		compacting bool
	}{
		{"PreCompact first", []Event{
			{Kind: CompactAsked, Request: askedID, By: "lead"},
			{Kind: PreCompact},
			{Kind: CompactRefused, Request: askedID, Started: true},
		}, false},
		{"PreCompact late", []Event{
			{Kind: CompactAsked, Request: askedID, By: "lead"},
			{Kind: CompactRefused, Request: askedID, Started: true},
			{Kind: PreCompact},
		}, false},
		{"refused before it started", []Event{
			{Kind: PreCompact},
			{Kind: CompactAsked, Request: askedID, By: "lead"},
			{Kind: CompactRefused, Request: askedID},
		}, true},
		{"another started between the word and the call", []Event{
			{Kind: CompactAsked, Request: askedID, By: "lead"},
			{Kind: PreCompact},
			{Kind: CompactRefused, Request: askedID},
		}, true},
		{"another started after the refusal", []Event{
			{Kind: CompactAsked, Request: askedID, By: "lead"},
			{Kind: CompactRefused, Request: askedID},
			{Kind: PreCompact},
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var f folding
			f.send(Event{Kind: SessionStart})
			for _, event := range tc.events {
				f.send(event)
			}
			if snapshot := f.state.snapshot(); *snapshot.Compacting != tc.compacting || *snapshot.Compactions != 0 {
				t.Fatalf("compacting %v, count %v; want compacting %v", *snapshot.Compacting, *snapshot.Compactions, tc.compacting)
			}
		})
	}
	// The late PreCompact is swallowed once: the next compaction's shows.
	var f folding
	for _, event := range []Event{
		{Kind: SessionStart},
		{Kind: CompactAsked, Request: askedID, By: "lead"},
		{Kind: CompactRefused, Request: askedID, Started: true},
		{Kind: PreCompact},
		{Kind: PreCompact},
	} {
		f.send(event)
	}
	if !*f.state.snapshot().Compacting {
		t.Fatal("the next compaction's PreCompact was swallowed too")
	}
}
