package workmode

import (
	"strings"
	"testing"
)

func TestDefaultsCycleAndInstructions(t *testing.T) {
	if got := (Config{}).Start(); got != Advisor {
		t.Fatalf("no start: %s", got)
	}
	if got := (Config{StartMode: Build}).Start(); got != Build {
		t.Fatalf("template start: %s", got)
	}
	// The built-ins cycle in the design's order, then the custom names sorted,
	// and an overridden built-in keeps its built-in place.
	catalog := Inventory(map[string]Mode{
		"review": {Name: "review"}, "advisor": {Name: "advisor"}, "build": {Name: "build"},
		"process": {Name: "process"}, "zebra": {Name: "zebra"}, "audit": {Name: "audit"},
	})
	var order []string
	for _, value := range catalog {
		order = append(order, value.Name)
	}
	if got := strings.Join(order, ","); got != "review,advisor,build,process,audit,zebra" {
		t.Fatalf("inventory: %s", got)
	}
	mode := Review
	for _, want := range []string{Advisor, Build, Process, "audit", "zebra", Review} {
		mode = Next(mode, catalog)
		if mode != want {
			t.Fatal(mode, want)
		}
	}
	if got := Next("gone", catalog); got != Review {
		t.Fatal("an unknown observed mode does not return to the first", got)
	}

	builtins := BuiltIns()
	for modeName, words := range map[string][]string{
		Review:  {"Report problems with precise files and lines", "leave what you review as it is", "Git metadata", "coordination remain allowed"},
		Advisor: {"concrete recommendations", "do not make product edits", "Git metadata", "coordination remain allowed"},
		Build:   {"named scope", "verify", "review and commit discipline", "no Git authority", "evidence and assumptions"},
		Process: {"procedure", "precondition", "end state", "coordination remain allowed"},
	} {
		value, ok := builtins[modeName]
		if !ok {
			t.Fatalf("no built-in %s", modeName)
		}
		text := Instructions(value, false)
		if !strings.HasPrefix(text, "Runtime work mode: "+modeName+". Your role, permissions and the project's rules still apply. ") {
			t.Errorf("%s lacks the mode header: %q", modeName, text[:80])
		}
		for _, word := range words {
			if !strings.Contains(text, word) {
				t.Errorf("%s lacks %q", modeName, word)
			}
		}
		if strings.Contains(text, "do not write product code yourself") {
			t.Error("a non-main mode claims orchestration", modeName)
		}
		if !strings.Contains(Instructions(value, true), "do not write product code yourself") {
			t.Error("main loses orchestration", modeName)
		}
	}
}

func TestOverlayAndValidation(t *testing.T) {
	user := Config{StartMode: Advisor, Modes: map[string]Fields{Review: {Model: "p/m", Effort: "high"}}}
	project := Config{Modes: map[string]Fields{Review: {Effort: "low"}, Build: {Model: "p/b"}}}
	template := Config{StartMode: Build, Modes: map[string]Fields{Review: {Effort: "medium"}}}
	got := user.Overlay(project).Overlay(template)
	if got.Start() != Build || got.Modes[Review] != (Fields{Model: "p/m", Effort: "medium"}) || got.Modes[Build].Model != "p/b" {
		t.Fatalf("%+v", got)
	}
	for _, c := range []Config{
		{StartMode: "plan mode"},
		{Modes: map[string]Fields{Review: {Model: "bare"}}},
		{Modes: map[string]Fields{Advisor: {Effort: "extreme"}}},
	} {
		if c.Check() == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
	if got.Check() != nil {
		t.Fatal(got.Check())
	}
	known := map[string]Mode{Advisor: {Name: Advisor}, Review: {Name: Review}}
	if err := (Config{StartMode: "plan"}).Unknown(known); err == nil || !strings.Contains(err.Error(), `"plan"`) || !strings.Contains(err.Error(), "advisor") {
		t.Fatalf("an unknown start mode: %v", err)
	}
	if err := (Config{Modes: map[string]Fields{"bogus": {}}}).Unknown(known); err == nil || !strings.Contains(err.Error(), `"bogus"`) {
		t.Fatalf("an unknown configured mode: %v", err)
	}
	if err := (Config{StartMode: Review, Modes: map[string]Fields{Advisor: {Effort: "low"}}}).Unknown(known); err != nil {
		t.Fatal(err)
	}
}
