package session

import (
	"testing"
	"time"
)

func text(value string) *string { return &value }

func count(value int64) *int64 { return &value }

var live = Reading{Live: true, Counted: true}

// The effort shown is the one whose source changed last: Claude Code's status
// line follows /effort before a request, and a request's own effort wins once
// it differs from that source's last, while one repeating it does not. Before
// any, a sample's own effort; before that, the launch's, marked. A reload,
// whose samples have seen nothing yet, keeps what was seen.
func TestTheEffortShownIsTheSourceThatChangedLast(t *testing.T) {
	o := NewObserver(ObserverConfig{Epoch: "e", Effort: "high"})
	if got := o.Snapshot(live); got.Effort == nil || *got.Effort != "high" || got.EffortSource != EffortFromLaunch {
		t.Fatalf("before anything: %v %q", got.Effort, got.EffortSource)
	}
	o.Sample(Sample{Thread: "c", Activity: "idle", Settings: &Settings{Effort: text("xhigh")}})
	if got := o.Snapshot(live); got.Effort == nil || *got.Effort != "xhigh" || got.EffortSource != "" {
		t.Fatalf("a sample's: %v %q", got.Effort, got.EffortSource)
	}
	for step, round := range []struct{ line, request, want string }{
		{request: "medium", want: "medium"},
		{line: "medium", want: "medium"},
		{line: "max", want: "max"},
		{request: "medium", want: "max"},
		{request: "max", want: "max"},
		{request: "low", want: "low"},
		{line: "max", want: "low"},
		{line: "high", want: "high"},
	} {
		if round.request != "" {
			o.Effort("request", round.request)
		}
		if round.line != "" {
			o.Effort("line", round.line)
		}
		if got := o.Snapshot(live); got.Effort == nil || *got.Effort != round.want || got.EffortSource != "" {
			t.Fatalf("step %d: effort %v (%q), want %q", step, got.Effort, got.EffortSource, round.want)
		}
	}
	o.Sample(Sample{Thread: "c", Activity: "idle", Settings: &Settings{}})
	if got := o.Snapshot(live); got.Effort == nil || *got.Effort != "high" || got.EffortSource != "" {
		t.Fatalf("after a sample without one: %v %q", got.Effort, got.EffortSource)
	}
}

// A sample is fresh while the source it came from is live and, where the
// source expires, until it is older than that; a part it did not carry was
// never observed. A stale one keeps its values, marked.
func TestASampleIsFreshWhileItsSourceIsLiveAndUntilItExpires(t *testing.T) {
	o := NewObserver(ObserverConfig{Epoch: "e", Expiry: 100 * time.Millisecond})
	if got := o.Snapshot(live); got.Fresh || got.Selection != "unknown" || got.Coverage != "unknown" || got.Compactions != nil {
		t.Fatalf("before a sample: %+v", got)
	}
	o.Sample(Sample{Thread: "c", Activity: "working"})
	got := o.Snapshot(live)
	if !got.Fresh || !got.ActivityFresh || got.ContextFresh || got.SettingsFresh || got.ContextAt != nil || got.SettingsAt != nil || got.Selection != "ready" || got.Interruptions != InterruptionsObserved {
		t.Fatalf("an activity alone: %+v", got)
	}
	o.Sample(Sample{Thread: "c", Activity: "working", Context: &Context{}, Settings: &Settings{Model: text("m")}})
	if got := o.Snapshot(live); !got.Fresh || !got.ActivityFresh || !got.ContextFresh || !got.SettingsFresh {
		t.Fatalf("a whole sample: %+v", got)
	}
	if got := o.Snapshot(Reading{}); got.Fresh || got.ActivityFresh || got.ContextFresh || got.SettingsFresh || *got.Activity != "working" || *got.Model != "m" {
		t.Fatalf("from a source gone: %+v", got)
	}
	time.Sleep(150 * time.Millisecond)
	if got := o.Snapshot(live); got.Fresh || got.ActivityFresh {
		t.Fatalf("past the expiry: %+v", got)
	}
	o.Thread("next")
	if got := o.Snapshot(live); got.Thread != "next" || got.Fresh {
		t.Fatalf("a new conversation: %q fresh %v", got.Thread, got.Fresh)
	}
}

// The context's counts are known only when they can be; a window the launch
// set below the model's is the limit shown, and the percent is of it.
func TestTheLaunchWindowIsTheLimitAndTheFillIsOfIt(t *testing.T) {
	percent := func(value int) *int { return &value }
	for _, c := range []struct {
		window  int64
		context Context
		want    [3]int64 // used, window, percent; -1 for unknown
	}{
		{0, Context{Used: count(500), Window: count(1000), Percent: percent(50)}, [3]int64{500, 1000, 50}},
		{0, Context{Used: count(-1), Window: count(1000), Percent: percent(50)}, [3]int64{-1, 1000, -1}},
		{0, Context{Used: count(500), Window: count(0), Percent: percent(50)}, [3]int64{500, -1, -1}},
		{0, Context{Used: count(500), Window: count(1000), Percent: percent(101)}, [3]int64{500, 1000, -1}},
		{400, Context{Used: count(100), Window: count(1000), Percent: percent(10)}, [3]int64{100, 400, 25}},
		{400, Context{Used: count(900), Window: count(1000), Percent: percent(90)}, [3]int64{900, 400, 100}},
		{4000, Context{Used: count(500), Window: count(1000), Percent: percent(50)}, [3]int64{500, 1000, 50}},
		{400, Context{}, [3]int64{-1, 400, -1}},
	} {
		o := NewObserver(ObserverConfig{Epoch: "e", Window: c.window})
		o.Sample(Sample{Thread: "c", Activity: "idle", Context: &c.context})
		s := o.Snapshot(live)
		var got [3]int64
		for i, value := range []*int64{s.ContextUsed, s.ContextWindow, nil} {
			got[i] = -1
			if value != nil {
				got[i] = *value
			}
		}
		if s.FilledPercent != nil {
			got[2] = int64(*s.FilledPercent)
		}
		if got != c.want {
			t.Errorf("launch window %d, %+v: %v, want %v", c.window, c.context, got, c.want)
		}
	}
}

// The status line's name, thinking and cache life stand over a sample's,
// across samples; without them a sample's own are shown.
func TestStandingFactsStandOverTheSamples(t *testing.T) {
	o := NewObserver(ObserverConfig{Epoch: "e"})
	off := false
	o.Sample(Sample{Thread: "c", Activity: "idle", Settings: &Settings{ModelName: text("Sampled"), Thinking: &off}, CacheLife: &CacheLife{TTL: "5m"}})
	if got := o.Snapshot(live); *got.ModelName != "Sampled" || *got.Thinking || got.CacheLife.TTL != "5m" {
		t.Fatalf("the sample's: %+v", got)
	}
	o.ModelName("Told")
	o.ModelName("")
	o.Thinking(true)
	o.CacheLife(CacheLife{TTL: "1h", Warm: true})
	o.Sample(Sample{Thread: "c", Activity: "idle", Settings: &Settings{ModelName: text("Sampled"), Thinking: &off}})
	if got := o.Snapshot(live); *got.ModelName != "Told" || !*got.Thinking || got.CacheLife.TTL != "1h" {
		t.Fatalf("the standing ones: %+v", got)
	}
}

// Compactions are counted from one source at a time, by its running count or
// by numbered ends, each once; a source that follows another leaves the count
// partial. The count waits for a sample and for the run to count, the cues
// and outcomes only for the latter, and an outcome is kept once per request.
func TestCompactionsAreCountedOnceFromEachSource(t *testing.T) {
	o := NewObserver(ObserverConfig{Epoch: "e"})
	o.Compacted("load-1", 0, "", "")
	o.Compacted("load-1", 1, "lead", "r1")
	o.Compacted("load-1", 1, "lead", "r1")
	o.Outcome(CompactionOutcome{Request: "r1", RequestedBy: "lead", Outcome: "done"})
	o.Outcome(CompactionOutcome{Request: "r1", RequestedBy: "lead", Outcome: "failed"})
	o.Outcome(CompactionOutcome{Request: "r2", Outcome: "done"})
	if got := o.Snapshot(live); got.Compactions != nil || len(got.CompactionEvents) != 1 || len(got.CompactionOutcomes) != 1 || got.CompactionOutcomes[0].Outcome != "done" {
		t.Fatalf("before a sample: %v %+v %+v", got.Compactions, got.CompactionEvents, got.CompactionOutcomes)
	}
	o.Sample(Sample{Thread: "c", Activity: "idle"})
	if got := o.Snapshot(Reading{Live: true}); got.Compactions != nil || got.CompactionEvents != nil || got.Coverage != "unknown" {
		t.Fatalf("uncounted: %+v", got)
	}
	o.Compacted("load-1", 3, "", "")
	if got := o.Snapshot(live); *got.Compactions != 3 || got.Coverage != "observed" || len(got.CompactionEvents) != 2 || got.CompactionEvents[1].Sequence != 3 {
		t.Fatalf("one source: %v %q %+v", *got.Compactions, got.Coverage, got.CompactionEvents)
	}
	o.Compacted("load-2", 1, "", "")
	for _, sequence := range []uint64{5, 5, 4, 6} {
		o.Compaction("link", sequence, "", "")
	}
	o.Compaction("link", 0, "", "")
	if got := o.Snapshot(live); *got.Compactions != 7 || got.Coverage != "partial" {
		t.Fatalf("sources after it: %v %q", *got.Compactions, got.Coverage)
	}
	for range 2 * maxCompactionEvents {
		o.Compaction("link", 0, "", "")
	}
	if got := o.Snapshot(live); len(got.CompactionEvents) != maxCompactionEvents || got.CompactionEvents[maxCompactionEvents-1].Sequence != *got.Compactions {
		t.Fatalf("%d cues, the last %d", len(got.CompactionEvents), got.CompactionEvents[len(got.CompactionEvents)-1].Sequence)
	}
	p := NewObserver(ObserverConfig{Epoch: "e"})
	p.Sample(Sample{Thread: "c", Activity: "idle"})
	p.Partial()
	if got := p.Snapshot(live); *got.Compactions != 0 || got.Coverage != "partial" {
		t.Fatalf("a drop: %v %q", *got.Compactions, got.Coverage)
	}
}
