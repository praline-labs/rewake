package workmode

import (
	"strings"
	"testing"
)

func TestDefaultsCycleAndStances(t *testing.T) {
	for kind, want := range map[string]string{"": Advisor, "analysis": Advisor, "process": Advisor, "custom": Advisor, "review": Review, "code": Build} {
		if got := (Config{}).Start(kind); got != want {
			t.Errorf("%q: %s", kind, got)
		}
		if got := (Config{StartMode: Review, TemplateStart: true}).Start(kind); got != Review {
			t.Errorf("override %q: %s", kind, got)
		}
	}
	for kind, want := range map[string]string{"review": Review, "code": Build, "": Advisor, "analysis": Advisor, "process": Advisor} {
		if got := (Config{StartMode: Advisor}).Start(kind); got != want {
			t.Errorf("global displaced type %q: %s", kind, got)
		}
	}
	if got := (Config{StartMode: Review}).Start("code"); got != Build {
		t.Fatal("global review displaced code", got)
	}
	if got := (Config{StartMode: Build}).Start("analysis"); got != Build {
		t.Fatal("global fallback ignored", got)
	}
	mode := Review
	for _, want := range []string{Advisor, Build, Review} {
		mode = Next(mode)
		if mode != want {
			t.Fatal(mode, want)
		}
	}
	for mode, words := range map[string][]string{
		Review:  {"correctness and regression", "precise files and lines", "Do not edit the code under review", "Git metadata", "coordination remain allowed"},
		Advisor: {"concrete recommendations", "Do not make product edits", "Git metadata", "coordination remain allowed"},
		Build:   {"named scope", "verify", "review and commit discipline", "no Git authority", "evidence and assumptions"},
	} {
		text := Instructions(mode, false)
		for _, word := range words {
			if !strings.Contains(text, word) {
				t.Errorf("%s lacks %q", mode, word)
			}
		}
		if !strings.Contains(Instructions(mode, true), "do not write product code yourself") {
			t.Error("main loses orchestration", mode)
		}
	}
}

func TestOverlayAndValidation(t *testing.T) {
	user := Config{StartMode: Advisor, Modes: Modes{Review: Fields{Model: "p/m", Effort: "high"}}}
	project := Config{Modes: Modes{Review: Fields{Effort: "low"}, Build: Fields{Model: "p/b"}}}
	template := Config{StartMode: Build, Modes: Modes{Review: Fields{Effort: "medium"}}}
	got := user.Overlay(project).Overlay(template)
	if got.Start("") != Build || got.Modes.Review != (Fields{Model: "p/m", Effort: "medium"}) || got.Modes.Build.Model != "p/b" {
		t.Fatalf("%+v", got)
	}
	for _, c := range []Config{{StartMode: "plan"}, {Modes: Modes{Review: Fields{Model: "bare"}}}, {Modes: Modes{Advisor: Fields{Effort: "extreme"}}}} {
		if c.Check() == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
	if got.Check() != nil {
		t.Fatal(got.Check())
	}
}
