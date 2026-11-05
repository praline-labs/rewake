package launch

import "github.com/praline-labs/rewake/internal/core/workmode"

// Modes gives a launch the merged global configuration and its selected
// template's overlay. Templates themselves still replace one another whole.
func (s *Set) Modes(template string) workmode.Config {
	c := s.pi
	if value, _, ok := s.Lookup(template); ok {
		c = c.Overlay(value.Pi)
		c.TemplateStart = value.Pi.StartMode != ""
	}
	return c
}
