package workmode

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Fields optionally select a mode's model and thinking level.
type Fields struct {
	Model  string `toml:"model" json:"model,omitempty"`
	Effort string `toml:"effort" json:"effort,omitempty"`
}

// Config is a launch's mode configuration: the mode it starts in, the
// per-mode model and thinking level, and the catalog resolved where the
// mode files were read. It is merged field by field, user then project then
// the selected template.
type Config struct {
	StartMode string            `toml:"start_mode" json:"startMode,omitempty"`
	Modes     map[string]Fields `toml:"modes" json:"modes,omitempty"`
	// Catalog is the modes this launch may start in and switch to, the
	// built-ins in cycle order then custom names sorted. It is not a TOML
	// setting: it is resolved where the mode files are read, and travels with
	// the configuration because the worker's checkout need not hold them.
	Catalog []Mode `toml:"-" json:"catalog,omitempty"`
}

// Check refuses invalid values before a launch can apply them: a mode name is
// a name, and a model an exact provider/model-id.
func (c Config) Check() error {
	if c.StartMode == "plan" {
		return errors.New(NoPlan)
	}
	if c.StartMode != "" && !ValidName(c.StartMode) {
		return fmt.Errorf("start_mode = %q cannot name a mode: letters, digits, dot, dash or underscore, not starting with a dot or dash", c.StartMode)
	}
	for modeName, f := range c.Modes {
		if modeName == "plan" {
			return errors.New(NoPlan)
		}
		if !ValidName(modeName) {
			return fmt.Errorf("modes.%q cannot name a mode: letters, digits, dot, dash or underscore, not starting with a dot or dash", modeName)
		}
		if f.Model != "" {
			provider, id, ok := strings.Cut(f.Model, "/")
			if !ok || provider == "" || id == "" || strings.ContainsAny(f.Model, "\r\n\t ") {
				return fmt.Errorf("the model of mode %q must be an exact provider/model-id", modeName)
			}
		}
		switch f.Effort {
		case "", "off", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return fmt.Errorf("the effort of mode %q must be off, minimal, low, medium, high, xhigh or max", modeName)
		}
	}
	return nil
}

// Unknown refuses a configured mode the catalog does not hold: a typo would
// otherwise apply nothing and start the mode it meant to replace.
func (c Config) Unknown(known map[string]Mode) error {
	var missing []string
	for modeName := range c.Modes {
		if _, ok := known[modeName]; !ok {
			missing = append(missing, modeName)
		}
	}
	if c.StartMode != "" {
		if _, ok := known[c.StartMode]; !ok {
			missing = append(missing, c.StartMode)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("names %s, which the mode catalog does not hold; available: %s",
		strings.Join(quoteAll(missing), ", "), strings.Join(Names(known), ", "))
}

func quoteAll(values []string) []string {
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = fmt.Sprintf("%q", value)
	}
	return out
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

// Overlay merges individual configured fields without inventing defaults. The
// catalog is not merged: it is resolved once, from the files of this launch.
func (c Config) Overlay(over Config) Config {
	if over.StartMode != "" {
		c.StartMode = over.StartMode
	}
	if len(over.Modes) > 0 && c.Modes == nil {
		c.Modes = map[string]Fields{}
	}
	for modeName, fields := range over.Modes {
		c.Modes[modeName] = mergeFields(c.Modes[modeName], fields)
	}
	return c
}

// Start is the mode a launch starts in: the merged start_mode, else advisor.
// There is no type-derived default any more, so a template that names no mode
// starts in advisor.
func (c Config) Start() string {
	if c.StartMode != "" {
		return c.StartMode
	}
	return Advisor
}

// Fields leaves unset destination selections empty so native values survive.
func (c Config) Fields(modeName string) Fields { return c.Modes[modeName] }
