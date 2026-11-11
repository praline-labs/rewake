package pi

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
)

// catalog is the built-ins in the design's order and one custom mode,
// resolved as space open resolves them.
func catalog(t *testing.T) []workmode.Mode {
	t.Helper()
	builtins := workmode.BuiltIns()
	audit := workmode.Mode{Name: "audit", Text: "AUDIT", Source: workmode.User}
	return workmode.Inventory(map[string]workmode.Mode{
		workmode.Review: builtins[workmode.Review], workmode.Advisor: builtins[workmode.Advisor],
		workmode.Build: builtins[workmode.Build], workmode.Process: builtins[workmode.Process], "audit": audit,
	})
}

func TestStartingModePrecedesDefaultsButNotExplicitFlags(t *testing.T) {
	c := workmode.Config{Modes: map[string]workmode.Fields{workmode.Build: {Model: "p/m", Effort: "high"}}}
	for _, args := range [][]string{nil, {"--model", "typed", "--thinking", "low"}, {"--models", "scope:medium"}} {
		got := harness{}.StartModeArgs(args, c, workmode.Build, false, nil)
		if len(args) > 0 && strings.Join(got, " ") != strings.Join(args, " ") {
			t.Errorf("explicit %v became %v", args, got)
		}
		if len(args) == 0 && (!sameFlag(got, "--model", "p/m") || !sameFlag(got, "--thinking", "high")) {
			t.Fatal(got)
		}
	}
	// Template flags yield before typed flags are merged; terminator data stays.
	got := harness{}.StartModeArgs([]string{"--model", "default", "--thinking", "low", "--", "--model", "literal"}, c, workmode.Build, true, nil)
	if !sameFlag(got, "--model", "p/m") || !sameFlag(got, "--thinking", "high") || !strings.HasSuffix(strings.Join(got, " "), "-- --model literal") {
		t.Fatal(got)
	}
	// An explicit model scope replaces the template's model selection; an
	// explicit model thinking suffix must not inherit a mode's --thinking.
	for _, typed := range [][]string{{"--models", "p/scope:low"}, {"--model", "p/typed:medium"}} {
		base := harness{}.StartModeArgs([]string{"--model", "alias", "--thinking", "off"}, c, workmode.Build, true, typed)
		if adapter.HasFlag(base, "--model") || adapter.HasFlag(base, "--thinking") {
			t.Fatalf("lower-priority selection survived %v: %v", typed, base)
		}
		base = append(base, typed...)
		selected := initialMode(adapter.LaunchRequest{Modes: c, Args: base, Mode: workmode.BuiltIns()[workmode.Build]})
		if selected.Model != "" || selected.Effort != "" {
			t.Fatalf("initial mode overrides typed selection: %+v", selected)
		}
	}
	request := adapter.LaunchRequest{Modes: c, Role: role.Main, Mode: workmode.BuiltIns()[workmode.Build]}
	p := initialMode(request)
	if p.Name != "build" || p.Model != "m" || p.Provider != "p" || p.Effort != "high" || !strings.Contains(p.Instructions, "orchestrator") {
		t.Fatalf("%+v", p)
	}
	request.Args = []string{"--model", "typed:medium"}
	p = initialMode(request)
	if p.Model != "" || p.Effort != "" {
		t.Fatalf("overrode explicit suffix: %+v", p)
	}
}

func TestModePlansCycleOnlyIdleAndProjectOnlyObservedMode(t *testing.T) {
	m := mirror.New()
	build := workmode.BuiltIns()[workmode.Build]
	rig := newRig(t, func(r *run) {
		r.mirror = m
		r.initial = planMode(workmode.BuiltIns()[workmode.Review], workmode.Fields{}, false)
		r.modes = workmode.Config{
			Modes:   map[string]workmode.Fields{workmode.Build: {Model: "p/second", Effort: "high"}},
			Catalog: catalog(t),
		}
	})
	l, hello := hello(t, rig, func(l *link, got frame) { l.answer(got, frame{OK: true}) })
	if hello.Mode == nil || hello.Mode.Name != "review" {
		t.Fatal(hello.Mode)
	}
	// The design's order: review, advisor, build, process, the custom name,
	// then back to review.
	for i, mode := range []string{"review", "advisor", "build", "process", "audit"} {
		request := frame{Op: "mode-cycle", Thread: "conversation", State: &sample{Sequence: uint64(i + 2), Idle: true, Native: nativeOf(map[string]any{"workMode": mode})}}
		got, err := l.ask(t.Context(), request)
		if err != nil || !got.OK || got.Mode == nil || got.Mode.Name != workmode.Next(mode, rig.run.modes.Catalog) {
			t.Fatalf("%+v %v", got, err)
		}
		if got.Mode.Name == "build" && (got.Mode.Model != "second" || got.Mode.Effort != "high" || !strings.Contains(got.Mode.Instructions, build.Text)) {
			t.Fatal(got.Mode)
		}
		if got.Mode.Name == "review" && (got.Mode.Model != "" || got.Mode.Effort != "") {
			t.Fatal("unset destination filled", got.Mode)
		}
		if got.Mode.Name == "audit" && !strings.Contains(got.Mode.Instructions, "AUDIT") {
			t.Fatalf("a custom mode has no text: %+v", got.Mode)
		}
		// Planning is not observing a completed native switch.
		state := rig.run.SessionState()
		if state.Mode == nil || *state.Mode != mode {
			t.Fatalf("unobserved destination shown: %+v", state.Mode)
		}
		begin := m.Snapshot()[1]
		if begin.WorkMode == nil || string(*begin.WorkMode) != mode || begin.Mode != nil {
			t.Fatalf("face differs: %+v", begin)
		}
	}
	busy, err := l.ask(t.Context(), frame{Op: "mode-cycle", Thread: "conversation", State: &sample{Sequence: 8, Idle: false, Native: nativeOf(map[string]any{"workMode": "build"})}})
	if err != nil || busy.OK || !strings.Contains(busy.Error, "busy") || busy.Mode != nil {
		t.Fatalf("busy %+v %v", busy, err)
	}
	if mode := rig.run.SessionState().Mode; mode == nil || *mode != "build" {
		t.Fatal("busy changed mode", mode)
	}
	if process, ok := workmode.Find(rig.run.modes.Catalog, "process"); !ok || process.Text == "" {
		t.Fatal("the catalog lost process")
	}
}
