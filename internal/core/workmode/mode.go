// Package workmode describes runtime working stances, not permissions or roles.
package workmode

import (
	"fmt"
	"strings"
)

// These are the only runtime stances; they are not permission modes.
const (
	Review  = "review"
	Advisor = "advisor"
	Build   = "build"
)

// Fields optionally select the destination mode's model and thinking level.
type Fields struct {
	Model  string `toml:"model" json:"model,omitempty"`
	Effort string `toml:"effort" json:"effort,omitempty"`
}

// Modes is a closed catalog: unknown TOML keys must remain errors.
type Modes struct {
	Review  Fields `toml:"review" json:"review"`
	Advisor Fields `toml:"advisor" json:"advisor"`
	Build   Fields `toml:"build" json:"build"`
}

// Config is merged field by field, user then project then selected template.
type Config struct {
	StartMode string `toml:"start_mode" json:"startMode,omitempty"`
	Modes     Modes  `toml:"modes" json:"modes"`
	// TemplateStart records the source of StartMode, not a TOML setting:
	// a global start cannot displace a review/code type default.
	TemplateStart bool `toml:"-" json:"templateStart,omitempty"`
}

// Valid keeps unrecognized runtime values out of observed state.
func Valid(mode string) bool { return mode == Review || mode == Advisor || mode == Build }

// Check refuses invalid values before a launch can apply them.
func (c Config) Check() error {
	if c.StartMode != "" && !Valid(c.StartMode) {
		return fmt.Errorf("start_mode must be review, advisor or build")
	}
	for _, f := range []Fields{c.Modes.Review, c.Modes.Advisor, c.Modes.Build} {
		if f.Model != "" {
			provider, id, ok := strings.Cut(f.Model, "/")
			if !ok || provider == "" || id == "" || strings.ContainsAny(f.Model, "\r\n\t ") {
				return fmt.Errorf("a mode model must be an exact provider/model-id")
			}
		}
		switch f.Effort {
		case "", "off", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return fmt.Errorf("a mode effort must be off, minimal, low, medium, high, xhigh or max")
		}
	}
	return nil
}

func mergeFields(base, over Fields) Fields {
	if over.Model != "" {
		base.Model = over.Model
	}
	if over.Effort != "" {
		base.Effort = over.Effort
	}
	return base
}

// Overlay merges individual configured fields without inventing defaults.
func (c Config) Overlay(over Config) Config {
	if over.StartMode != "" {
		c.StartMode = over.StartMode
	}
	c.Modes.Review = mergeFields(c.Modes.Review, over.Modes.Review)
	c.Modes.Advisor = mergeFields(c.Modes.Advisor, over.Modes.Advisor)
	c.Modes.Build = mergeFields(c.Modes.Build, over.Modes.Build)
	return c
}

// Start keeps a global fallback below type discipline and a template above it.
func (c Config) Start(kind string) string {
	if c.TemplateStart && c.StartMode != "" {
		return c.StartMode
	}
	switch kind {
	case "review":
		return Review
	case "code":
		return Build
	}
	if c.StartMode != "" {
		return c.StartMode
	}
	return Advisor
}

// Fields leaves unset destination selections empty so native values survive.
func (c Config) Fields(mode string) Fields {
	switch mode {
	case Review:
		return c.Modes.Review
	case Advisor:
		return c.Modes.Advisor
	case Build:
		return c.Modes.Build
	}
	return Fields{}
}

// Next is the shared cycle for native and face requests.
func Next(mode string) string {
	switch mode {
	case Review:
		return Advisor
	case Advisor:
		return Build
	default:
		return Review
	}
}

// Instructions add a stance to the role and worker-type briefings. Nothing
// here grants Git authority or claims to enforce a read-only boundary.
func Instructions(mode string, main bool) string {
	text := "Runtime work mode: " + mode + ". Your role, permissions and the project's rules still apply. "
	switch mode {
	case Review:
		text += "Inspect the requested change against its brief and the project's rules. Find correctness and regression problems; cite precise files and lines. Run relevant checks when asked. Do not edit the code under review or change Git metadata. Normal rewake mail, brief comments and coordination remain allowed."
	case Advisor:
		text += "Answer, analyze and compare approaches with concrete recommendations. Do not make product edits or change Git metadata in this stance. Normal rewake mail, brief comments and coordination remain allowed."
	case Build:
		text += "Implement the brief in its named scope and verify the result. Follow the project's review and commit discipline; build grants no Git authority. Report evidence and assumptions honestly."
	}
	if main {
		text += " You remain the orchestrator in every mode: delegate implementation; do not write product code yourself."
	}
	return text
}
