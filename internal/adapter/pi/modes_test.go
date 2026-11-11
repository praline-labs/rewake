package pi

import (
	"encoding/json"
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

// The native status names the observed mode alone: it is a launch choice, so
// no key is claimed beside it.
func TestTheModeStatusNamesTheModeWithoutACycleKey(t *testing.T) {
	p := planMode(workmode.BuiltIns()[workmode.Build], workmode.Fields{}, false, false)
	if p.Status != "mode: build" {
		t.Fatalf("the status says %q", p.Status)
	}
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
		r.initial = planMode(workmode.BuiltIns()[workmode.Review], workmode.Fields{}, false, false)
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

// A session carries one behavioral section at a time: a switch replaces the
// previous mode's text instead of adding to it. Two defects of the old two
// axes fail here — a reviewer switched to build kept the review type's text
// beside build's stance (stacked, contradictory instructions), and a worker
// whose kind of work was process could start in advisor's stance.
func TestASwitchLeavesOneSectionAndProcessIsItsOwn(t *testing.T) {
	builtins := workmode.BuiltIns()
	markers := map[string]string{
		workmode.Review:  "Report problems with precise files and lines",
		workmode.Advisor: "do not make product edits",
		workmode.Build:   "Implement the brief in its named scope",
		workmode.Process: "Check each step's preconditions",
	}
	// The switch a reviewer makes: review's text does not survive into build,
	// and no second header rides along.
	built := planMode(builtins[workmode.Build], workmode.Fields{}, false, false)
	if strings.Count(built.Instructions, "Runtime work mode:") != 1 {
		t.Fatalf("build carries more than one behavioral section: %q", built.Instructions)
	}
	if strings.Contains(built.Instructions, markers[workmode.Review]) {
		t.Fatalf("review's text survived into build: %q", built.Instructions)
	}
	// Process's own discipline, never advisor's: the reason process is a mode.
	process := planMode(builtins[workmode.Process], workmode.Fields{}, false, false)
	if !strings.Contains(process.Instructions, markers[workmode.Process]) || strings.Contains(process.Instructions, markers[workmode.Advisor]) {
		t.Fatalf("process does not carry its own discipline: %q", process.Instructions)
	}
	// Every built-in, and a switch from each, carries exactly its own text.
	for _, from := range workmode.Inventory(builtins) {
		plan := planMode(from, workmode.Fields{}, false, false)
		if strings.Count(plan.Instructions, "Runtime work mode:") != 1 || !strings.Contains(plan.Instructions, from.Text) {
			t.Fatalf("%s lacks its one section: %q", from.Name, plan.Instructions)
		}
		next, ok := workmode.Find(workmode.Inventory(builtins), workmode.Next(from.Name, workmode.Inventory(builtins)))
		if !ok {
			t.Fatalf("no mode after %s", from.Name)
		}
		switched := planMode(next, workmode.Fields{}, false, false)
		for name, marker := range markers {
			if name != next.Name && strings.Contains(switched.Instructions, marker) {
				t.Fatalf("%s after %s carries %s's text: %q", next.Name, from.Name, name, switched.Instructions)
			}
		}
	}
	// No mode is inferred: a template that names none starts in advisor, and
	// process starts only by being named.
	if start := (workmode.Config{}).Start(); start != workmode.Advisor {
		t.Fatalf("an unnamed start is %q, not advisor", start)
	}
	if start := (workmode.Config{StartMode: workmode.Process}).Start(); start != workmode.Process {
		t.Fatalf("a process start became %q", start)
	}
}

// A wrapper handover and a reloaded extension meet the same JSON: the catalog
// arrives with every mode's text, so hello and a cycle carry a whole
// discipline rather than a bare header.
func TestAHelloAndACycleAfterHandoverCarryWholeDisciplines(t *testing.T) {
	modes := workmode.Config{Catalog: catalog(t), Modes: map[string]workmode.Fields{workmode.Build: {Effort: "high"}}}
	encoded, err := json.Marshal(modes)
	if err != nil {
		t.Fatal(err)
	}
	var handed workmode.Config
	if err := json.Unmarshal(encoded, &handed); err != nil {
		t.Fatal(err)
	}
	rig := newRig(t, func(r *run) {
		r.modes = handed
		r.mode = workmode.Process
	})
	l, greeted := hello(t, rig, func(l *link, got frame) { l.answer(got, frame{OK: true}) })
	if greeted.Mode == nil || greeted.Mode.Name != workmode.Process {
		t.Fatalf("hello %+v", greeted.Mode)
	}
	process := workmode.BuiltIns()[workmode.Process]
	if strings.Count(greeted.Mode.Instructions, "Runtime work mode:") != 1 || !strings.Contains(greeted.Mode.Instructions, process.Text) {
		t.Fatalf("hello after a handover carries no whole process discipline: %q", greeted.Mode.Instructions)
	}
	got, err := l.ask(t.Context(), frame{Op: "mode-cycle", Thread: "conversation", State: &sample{Sequence: 2, Idle: true, Native: nativeOf(map[string]any{"workMode": workmode.Process})}})
	if err != nil || !got.OK || got.Mode == nil {
		t.Fatalf("%+v %v", got, err)
	}
	// process -> the custom name -> review: the custom-only names follow the
	// built-ins, and each destination brings its own whole text.
	if got.Mode.Name != "audit" || strings.Count(got.Mode.Instructions, "Runtime work mode:") != 1 || !strings.Contains(got.Mode.Instructions, "AUDIT") {
		t.Fatalf("the cycle after a handover lost a custom mode's discipline: %+v", got.Mode)
	}
	got, err = l.ask(t.Context(), frame{Op: "mode-cycle", Thread: "conversation", State: &sample{Sequence: 3, Idle: true, Native: nativeOf(map[string]any{"workMode": "audit"})}})
	if err != nil || !got.OK || got.Mode == nil || got.Mode.Name != workmode.Review || !strings.Contains(got.Mode.Instructions, workmode.BuiltIns()[workmode.Review].Text) {
		t.Fatalf("the cycle back to review lost its discipline: %+v %v", got.Mode, err)
	}
	// A custom mode's own text survives the same boundary; it is a mode of the
	// person's, not a built-in name.
	audit, ok := workmode.Find(handed.Catalog, "audit")
	if !ok || audit.Text != "AUDIT" || !strings.Contains(workmode.Instructions(audit, false, false), "AUDIT") {
		t.Fatalf("a custom mode lost its text at the handover: %+v", audit)
	}
}

// Main's authorship policy is in every Go-planned mode section, including the
// one a reconnected extension receives at hello: the policy is neither a
// worker's nor a value the extension resolves for itself.
func TestTheAuthorshipPolicyReachesEveryPlanAndAHello(t *testing.T) {
	for _, mode := range workmode.Inventory(workmode.BuiltIns()) {
		name := mode.Name
		implementing := planMode(mode, workmode.Fields{}, true, true)
		if !strings.Contains(implementing.Instructions, "implementation is permitted only when the task, the mode and the project's rules allow it") ||
			strings.Contains(implementing.Instructions, "do not write product code yourself") {
			t.Errorf("%s: an implementing main's plan is %q", name, implementing.Instructions)
		}
		delegating := planMode(mode, workmode.Fields{}, true, false)
		if !strings.Contains(delegating.Instructions, "do not write product code yourself") {
			t.Errorf("%s: a delegating main's plan is %q", name, delegating.Instructions)
		}
		worker := planMode(mode, workmode.Fields{}, false, true)
		if strings.Contains(worker.Instructions, "implementation is permitted") || strings.Contains(worker.Instructions, "orchestrator") {
			t.Errorf("%s: a worker's plan carries main's policy: %q", name, worker.Instructions)
		}
	}
	rig := newRig(t, func(r *run) {
		r.modes = workmode.Config{Catalog: catalog(t)}
		r.mode = workmode.Process
		r.hostsSpaces, r.mainImplements = true, true
	})
	_, greeted := hello(t, rig, func(l *link, got frame) { l.answer(got, frame{OK: true}) })
	if greeted.Mode == nil || !strings.Contains(greeted.Mode.Instructions, "implementation is permitted only when the task, the mode and the project's rules allow it") {
		t.Fatalf("hello after a reconnect lost the policy: %+v", greeted.Mode)
	}
}

// The starting mode's selection is said once, by the mode and source, and
// never with the configured value: the catalog and the native strip show
// what was selected (docs/rules/launch.md, L2).
func TestAStartingModeSelectionIsSaidWithoutItsValue(t *testing.T) {
	request := adapter.LaunchRequest{
		Modes: workmode.Config{Modes: map[string]workmode.Fields{workmode.Build: {Model: "p/m", Effort: "high"}}},
		Mode:  workmode.BuiltIns()[workmode.Build],
	}
	notes := startingModeNotes(request, nil)
	if len(notes) != 1 || !strings.Contains(notes[0], `the starting mode "build" supplied the model and thinking level`) ||
		!strings.Contains(notes[0], "overrides the template's launch settings") || strings.Contains(notes[0], "p/m") {
		t.Fatalf("the note: %q", notes)
	}
	if notes := startingModeNotes(request, []string{"--model", "typed/model"}); len(notes) != 1 || strings.Contains(notes[0], "the model and") || !strings.Contains(notes[0], "thinking level") {
		t.Fatalf("an explicit model is not the mode's: %q", notes)
	}
	if notes := startingModeNotes(request, []string{"--model", "typed/model", "--thinking", "low"}); len(notes) != 0 {
		t.Fatalf("both explicit, yet: %q", notes)
	}
	if notes := startingModeNotes(adapter.LaunchRequest{Mode: workmode.BuiltIns()[workmode.Build]}, nil); len(notes) != 0 {
		t.Fatalf("an unset selection was said: %q", notes)
	}
}
