package launch

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
)

// AliasSettings are the portable settings a worker catalog shows. An omitted
// Apply means true; Context never becomes an argument.
type AliasSettings struct {
	Model   string `toml:"model"`
	Effort  string `toml:"effort"`
	Context string `toml:"context"`
	Apply   *bool  `toml:"apply"`
}

// Applied says whether model and effort become launch flags.
func (s AliasSettings) Applied() bool { return s.Apply == nil || *s.Apply }

// Err refuses a catalog that could silently omit workers from an unreadable
// file. Launches retain their existing advisory notes.
func (s *Set) Err() error {
	if len(s.problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(s.problems, "; "))
}

func (s *Set) unreadable(message string) {
	s.Notes = append(s.Notes, message)
	s.problems = append(s.problems, message)
}

// ExpandConfigured applies structured settings through the selected adapter,
// then uses the ordinary alias expansion, including its project-file checks
// and the precedence of explicitly typed flags. The stored alias stays intact.
func (s *Set) ExpandConfigured(argv []string, catalog adapter.Catalog, takesValue func(string) bool, forbidden []string) ([]string, error) {
	position := commandWord(argv, takesValue)
	if position < 0 {
		return argv, nil
	}
	name := argv[position]
	value, source, found := s.Lookup(name)
	if !found {
		return argv, nil
	}
	if err := value.check(name, source, catalog.IDs(), takesValue); err != nil {
		return nil, err
	}
	h, _ := catalog.Find(value.Harness)
	if value.Launch.Applied() && (value.Launch.Model != "" || value.Launch.Effort != "") {
		settings, ok := h.(adapter.LaunchSettingsHarness)
		if !ok {
			return nil, fmt.Errorf("alias %q: %s cannot apply launch settings; use launch.apply = false or harness arguments", name, h.Title())
		}
		flags := settings.LaunchSettings(value.Launch.Model, value.Launch.Effort)
		// Structured fields replace an older copy in args, even when args
		// ends with a prompt terminator. New flags still precede that tail.
		visible := adapter.BeforeTerminator(value.Args)
		kept := replaced(visible, flags, h.SingleUseFlags())
		value.Args = append(slices.Clone(kept), value.Args[len(visible):]...)
		value.Args = adapter.AddFlags(value.Args, flags...)
	}
	copySet := *s
	copySet.aliases = map[string]Alias{name: value}
	return copySet.Expand(argv, catalog.IDs(), takesValue, forbidden, func(id string) []adapter.Flag {
		selected, ok := catalog.Find(id)
		if !ok {
			return nil
		}
		return selected.SingleUseFlags()
	})
}

// MergeArguments applies the same single-use replacement as a direct alias
// launch. A terminator in the alias still makes later words prompt input.
func MergeArguments(base, typed []string, flags []adapter.Flag) []string {
	return append(slices.Clone(replaced(base, typed, flags)), typed...)
}
