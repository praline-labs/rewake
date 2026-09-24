package telemetry

import (
	"testing"
	"time"
)

// The variable reads as the harness reads it: plain, exponent and grouped
// digits; a leading count otherwise; nothing for a value that is no count or
// not above zero; and within the bounds of 100K and 1M.
func TestTheVariableReadsAsTheHarnessReadsIt(t *testing.T) {
	if got := envWindow(" 300000 "); got == nil || *got != 300000 {
		t.Errorf("a padded value reads as %v, want 300000", got)
	}
	for raw, want := range map[string]int64{
		"300000": 300000, "3e5": 300000, "300,000": 300000, "300_000": 300000,
		"300k": 100000, "50000": 100000, "2000000": 1000000, "150000.5": 150000,
		"": -1, "0": -1, "-5": -1, "abc": -1, "k300": -1,
		// Where a looser reading once parted from the harness's, found by review
		// on September 24, 2026: an exponent form must come out whole, and a
		// group is exactly three digits after one separator used throughout.
		"0.5": -1, ".5": -1, "1.5e-1": -1, "1.234567e5": -1,
		"300 000": 300000, "300\u00a0000": 300000, "300\u202f000": 300000,
		"3,00000": 100000, "300,000_000": 100000,
	} {
		got := envWindow(raw)
		switch {
		case want < 0 && got != nil:
			t.Errorf("envWindow(%q) = %d, want nothing", raw, *got)
		case want >= 0 && (got == nil || *got != want):
			t.Errorf("envWindow(%q) = %v, want %d", raw, got, want)
		}
	}
}

// The settings key and the flag take only what the harness takes, and the
// flag's auto is an answer of its own.
func TestTheKeyAndTheFlagTakeOnlyWhatTheHarnessTakes(t *testing.T) {
	for value, want := range map[float64]bool{400000: true, 100000: true, 1000000: true, 99999: false, 1000001: false, 250000.5: false} {
		if got := settingsWindow(&value); (got != nil) != want {
			t.Errorf("settingsWindow(%v) = %v, want taken %v", value, got, want)
		}
	}
	if got := ParseAutocompact(" AUTO "); got != (Autocompact{Set: true, Auto: true}) {
		t.Errorf("a padded AUTO reads as %+v", got)
	}
	for raw, want := range map[string]Autocompact{
		"auto": {Set: true, Auto: true},
		"500k": {Set: true, Window: 500000}, "1m": {Set: true, Window: 1000000}, "200": {Set: true, Window: 200000},
		"250000": {Set: true, Window: 250000}, "0.3m": {Set: true, Window: 300000},
		"150000.5": {Set: true, Window: 150000}, "300,000": {Set: true, Window: 300000},
		"50k": {}, "2m": {}, "99": {}, "soon": {}, "": {},
	} {
		if got := ParseAutocompact(raw); got != want {
			t.Errorf("ParseAutocompact(%q) = %+v, want %+v", raw, got, want)
		}
	}
}

// The variable comes first, then the flag — whose auto sets the key aside —
// then the key; without the plugin's word there is no limit at all.
func TestTheLimitFollowsTheHarnesssOrder(t *testing.T) {
	window := func(value int64) *int64 { return &value }
	both := &Limit{Env: window(300000), Settings: window(400000)}
	keyOnly := &Limit{Settings: window(400000)}
	for _, tc := range []struct {
		name  string
		limit *Limit
		flag  Autocompact
		want  int64
	}{
		{"the variable over the flag and the key", both, Autocompact{Set: true, Window: 500000}, 300000},
		{"the flag over the key", keyOnly, Autocompact{Set: true, Window: 500000}, 500000},
		{"the flag's auto sets the key aside", keyOnly, Autocompact{Set: true, Auto: true}, -1},
		{"the key alone", keyOnly, Autocompact{}, 400000},
		{"none of the three", &Limit{}, Autocompact{}, -1},
		{"no word from the plugin, a flag notwithstanding", nil, Autocompact{Set: true, Window: 500000}, -1},
	} {
		got := configured(tc.limit, tc.flag)
		if (tc.want < 0) != (got == nil) || (got != nil && *got != tc.want) {
			t.Errorf("%s: %v, want %d", tc.name, got, tc.want)
		}
	}
}

// The listing shows the limit as the window, with the percent of it; a limit
// above the model's window, or no limit, leaves the status line's numbers as
// they are.
func TestTheSnapshotShowsTheLimitAsTheWindow(t *testing.T) {
	now := time.Now()
	status := func(s *state, at int64, used, window int64, percent int) {
		s.apply(Event{At: at, Kind: StatusLine, Context: &Context{Used: &used, Window: &window, Percent: &percent}}, now)
	}
	ready := func(s *state, at int64, env string) {
		s.apply(Event{At: at, Kind: PluginReady, Limit: &Limit{Env: envWindow(env)}}, now)
	}
	check := func(name string, s *state, window int64, percent int) {
		t.Helper()
		snapshot := s.snapshot()
		if snapshot.ContextWindow == nil || *snapshot.ContextWindow != window || snapshot.FilledPercent == nil || *snapshot.FilledPercent != percent {
			t.Errorf("%s: window %v, percent %v; want %d, %d", name, snapshot.ContextWindow, snapshot.FilledPercent, window, percent)
		}
	}

	var capped state
	ready(&capped, 1, "300000")
	status(&capped, 2, 150000, 1000000, 15)
	check("under 300000", &capped, 300000, 50)

	var unlimited state
	ready(&unlimited, 1, "")
	status(&unlimited, 2, 150000, 1000000, 15)
	check("without a limit", &unlimited, 1000000, 15)

	var above state
	ready(&above, 1, "300000")
	status(&above, 2, 50000, 200000, 25)
	check("a limit above the model's window", &above, 200000, 25)

	var unheard state
	unheard.autocompact = ParseAutocompact("300k")
	status(&unheard, 2, 150000, 1000000, 15)
	check("without the plugin", &unheard, 1000000, 15)

	// A measure brings the limit read again; the newest one decides.
	var changed state
	ready(&changed, 1, "300000")
	status(&changed, 2, 150000, 1000000, 15)
	measure := int64(150000)
	changed.apply(Event{At: 3, Kind: SessionMeasure, Context: &Context{Used: &measure}, Limit: &Limit{Env: envWindow("150000")}}, now)
	check("after a measure with a new limit", &changed, 150000, 100)

	// A measure whose reads failed carries no limit, and the last one stands.
	changed.apply(Event{At: 4, Kind: SessionMeasure, Context: &Context{Used: &measure}}, now)
	check("after a measure without a limit", &changed, 150000, 100)
}
