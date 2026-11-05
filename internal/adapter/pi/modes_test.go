package pi

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
	"github.com/praline-labs/rewake/internal/core/worktype"
)

func TestStartingModePrecedesDefaultsButNotExplicitFlags(t *testing.T) {
	c := workmode.Config{Modes: workmode.Modes{Build: workmode.Fields{Model: "p/m", Effort: "high"}}}
	for _, args := range [][]string{nil, {"--model", "typed", "--thinking", "low"}, {"--models", "scope:medium"}} {
		got := harness{}.StartModeArgs(args, c, "code", false, nil)
		if len(args) > 0 && strings.Join(got, " ") != strings.Join(args, " ") {
			t.Errorf("explicit %v became %v", args, got)
		}
		if len(args) == 0 && (!sameFlag(got, "--model", "p/m") || !sameFlag(got, "--thinking", "high")) {
			t.Fatal(got)
		}
	}
	// Template flags yield before typed flags are merged; terminator data stays.
	got := harness{}.StartModeArgs([]string{"--model", "default", "--thinking", "low", "--", "--model", "literal"}, c, "code", true, nil)
	if !sameFlag(got, "--model", "p/m") || !sameFlag(got, "--thinking", "high") || !strings.HasSuffix(strings.Join(got, " "), "-- --model literal") {
		t.Fatal(got)
	}
	// An explicit model scope replaces the template's model selection; an
	// explicit model thinking suffix must not inherit a mode's --thinking.
	for _, typed := range [][]string{{"--models", "p/scope:low"}, {"--model", "p/typed:medium"}} {
		base := harness{}.StartModeArgs([]string{"--model", "alias", "--thinking", "off"}, c, "code", true, typed)
		if adapter.HasFlag(base, "--model") || adapter.HasFlag(base, "--thinking") {
			t.Fatalf("lower-priority selection survived %v: %v", typed, base)
		}
		base = append(base, typed...)
		selected := initialMode(adapter.LaunchRequest{Modes: c, Args: base, WorkType: worktype.Type{Name: "code"}})
		if selected.Model != "" || selected.Effort != "" {
			t.Fatalf("initial mode overrides typed selection: %+v", selected)
		}
	}
	request := adapter.LaunchRequest{Modes: c, Role: role.Main, WorkType: worktype.Type{Name: "code"}}
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
	rig := newRig(t, func(r *run) {
		r.mirror = m
		r.initial = planMode("review", workmode.Fields{}, false)
		r.modes = workmode.Config{Modes: workmode.Modes{Build: workmode.Fields{Model: "p/second", Effort: "high"}}}
	})
	l, hello := hello(t, rig, func(l *link, got frame) { l.answer(got, frame{OK: true}) })
	if hello.Mode == nil || hello.Mode.Name != "review" {
		t.Fatal(hello.Mode)
	}
	for i, mode := range []string{"review", "advisor", "build"} {
		request := frame{Op: "mode-cycle", Thread: "conversation", State: &sample{Sequence: uint64(i + 2), Idle: true, Native: nativeOf(map[string]any{"workMode": mode})}}
		got, err := l.ask(t.Context(), request)
		if err != nil || !got.OK || got.Mode == nil || got.Mode.Name != workmode.Next(mode) {
			t.Fatalf("%+v %v", got, err)
		}
		if got.Mode.Name == "build" && (got.Mode.Model != "second" || got.Mode.Effort != "high") {
			t.Fatal(got.Mode)
		}
		if got.Mode.Name == "review" && (got.Mode.Model != "" || got.Mode.Effort != "") {
			t.Fatal("unset destination filled", got.Mode)
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
}
