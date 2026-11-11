package pi

import (
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/workmode"
)

// modePlan is effective policy rendered by Go; the native bridge only applies
// its optional selections and publishes the observed result.
type modePlan struct {
	Name         string `json:"name"`
	Instructions string `json:"instructions"`
	Status       string `json:"status"`
	Provider     string `json:"provider,omitempty"`
	Model        string `json:"model,omitempty"`
	Effort       string `json:"effort,omitempty"`
	Busy         string `json:"busy"`
	Failed       string `json:"failed"`
}

// planMode renders one mode's complete behavioral section, the only one the
// session's requests carry: a switch replaces it, never adds another.
func planMode(mode workmode.Mode, fields workmode.Fields, main bool) *modePlan {
	p := &modePlan{
		Name: mode.Name, Instructions: workmode.Instructions(mode, main), Status: "mode: " + mode.Name + " (shift+tab)", Effort: fields.Effort,
		Busy: "work mode was not changed: the session is busy", Failed: "work mode was not changed: the configured model could not be selected",
	}
	if fields.Model != "" {
		p.Provider, p.Model, _ = strings.Cut(fields.Model, "/")
	}
	return p
}

// StartModeArgs runs before ordinary defaults. Workers overlay their template
// before the arguments explicitly typed after -- are merged.
func (harness) StartModeArgs(args []string, c workmode.Config, mode string, replace bool, typed []string) []string {
	f := c.Fields(mode)
	explicit := args
	if replace {
		explicit = typed
	}
	modelNamed := adapter.HasFlag(explicit, "--model") || adapter.HasFlag(explicit, "--models") || adapter.HasFlag(explicit, "--provider")
	effortNamed := thinkingNamed(explicit)
	if replace && modelNamed {
		args = dropModeFlags(args, "--model", "--models", "--provider")
	}
	if replace && effortNamed {
		args = dropModeFlags(args, "--thinking")
	}
	if f.Model != "" && !modelNamed {
		if replace {
			args = dropModeFlags(args, "--model", "--models", "--provider")
		}
		args = adapter.AddFlags(args, "--model", f.Model)
	}
	if f.Effort != "" && !effortNamed {
		if replace {
			args = dropModeFlags(args, "--thinking")
		}
		args = adapter.AddFlags(args, "--thinking", f.Effort)
	}
	return args
}

func dropModeFlags(args []string, flags ...string) []string {
	visible := adapter.BeforeTerminator(args)
	var kept []string
	for i := 0; i < len(visible); i++ {
		drop := false
		for _, flag := range flags {
			if _, separate, ok := adapter.MatchFlag(visible[i], flag); ok {
				drop = true
				if separate && i+1 < len(visible) {
					i++
				}
				break
			}
		}
		if !drop {
			kept = append(kept, visible[i])
		}
	}
	return append(kept, args[len(visible):]...)
}

// initialMode validates a configured startup mode and model through the
// registry too: pi's CLI may otherwise fuzzy-match a misspelled exact model id.
func initialMode(request adapter.LaunchRequest) *modePlan {
	mode := request.Mode
	f := request.Modes.Fields(mode.Name)
	args := request.Args
	if adapter.HasFlag(args, "--provider") || (adapter.HasFlag(args, "--model") || adapter.HasFlag(args, "--models")) && !sameFlag(args, "--model", f.Model) {
		f.Model = ""
	}
	if thinkingNamed(args) && !sameFlag(args, "--thinking", f.Effort) {
		f.Effort = ""
	}
	return planMode(mode, f, request.Role.Spaces)
}

func sameFlag(args []string, flag, value string) bool {
	values := adapter.FlagValues(args, flag)
	return value != "" && len(values) == 1 && values[0] == value
}

func (r *run) helloMode() *modePlan {
	r.mu.Lock()
	defer r.mu.Unlock()
	if value, ok := workmode.Find(r.modes.Catalog, r.mode); ok {
		return planMode(value, workmode.Fields{}, r.hostsSpaces)
	}
	return r.initial
}

func (r *run) cycleMode(l *link, request frame) frame {
	r.observe(l, request.Thread, request.State)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.observed.link != l || request.State == nil || !request.State.Idle || request.State.Compacting {
		return frame{Error: "work mode was not changed: the session is busy"}
	}
	if _, ok := workmode.Find(r.modes.Catalog, r.mode); !ok {
		return frame{Error: "work mode is not available yet"}
	}
	next := workmode.Next(r.mode, r.modes.Catalog)
	value, ok := workmode.Find(r.modes.Catalog, next)
	if !ok {
		return frame{Error: "work mode is not available yet"}
	}
	return frame{OK: true, Mode: planMode(value, r.modes.Fields(next), r.hostsSpaces)}
}

func (r *run) cycleThinking(l *link, request frame) frame {
	r.observe(l, request.Thread, request.State)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.observed.model == nil || !r.observed.model.Reasoning {
		return frame{Error: "the current model does not support thinking"}
	}
	return frame{OK: true, Levels: levelsOf(r.observed.model)}
}
