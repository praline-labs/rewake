package modhost

import (
	"encoding/json"
	"math"
	"strconv"
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
)

// What the mod read of the session comes as Claude Code answered it (the mods
// skill's SessionUsage and TurnStepResult), and is named and converted here,
// so a change of what the Статус shows is the wrapper's alone. Each part is
// decoded on its own: a part whose shape a release changed is unknown, and the
// rest still counts.

// projected is a state's native part as the session's sample takes it.
type projected struct {
	model      *string
	cwd        string
	context    *session.Context
	rateLimits []session.RateLimit
	// effort is what the last main-loop request asked for, "" when it named
	// none.
	effort string
	// cache is the last step's usage, nil where it carried none or had no
	// input: a placeholder, which leaves the last observation standing.
	cache *session.Cache
}

type nativeState struct {
	Model json.RawMessage `json:"model"`
	Cwd   json.RawMessage `json:"cwd"`
	Usage json.RawMessage `json:"usage"`
	Step  json.RawMessage `json:"step"`
}

type nativeUsage struct {
	Context    json.RawMessage `json:"context"`
	RateLimits json.RawMessage `json:"rateLimits"`
}

type nativeStep struct {
	Usage  json.RawMessage `json:"usage"`
	Effort json.RawMessage `json:"effort"`
}

// project reads a state's native part; nothing of a part it cannot read.
func project(raw json.RawMessage) projected {
	got := projected{context: &session.Context{}}
	var parts nativeState
	if json.Unmarshal(raw, &parts) != nil {
		return got
	}
	var model, cwd string
	if json.Unmarshal(parts.Model, &model) == nil && model != "" {
		got.model = &model
	}
	if json.Unmarshal(parts.Cwd, &cwd) == nil {
		got.cwd = cwd
	}
	var usage nativeUsage
	if json.Unmarshal(parts.Usage, &usage) == nil {
		got.context = contextOf(usage.Context)
		got.rateLimits = limitsOf(usage.RateLimits)
	}
	var step nativeStep
	if json.Unmarshal(parts.Step, &step) == nil {
		got.effort = effortOf(step.Effort)
		got.cache = cacheOf(step.Usage)
	}
	return got
}

// contextOf is the live window's figures, the status line's.
func contextOf(raw json.RawMessage) *session.Context {
	var context struct {
		Tokens  *int64 `json:"tokens"`
		Window  *int64 `json:"window"`
		Percent *int   `json:"percent"`
	}
	if json.Unmarshal(raw, &context) != nil {
		return &session.Context{}
	}
	return &session.Context{Used: context.Tokens, Window: context.Window, Percent: context.Percent}
}

// limitsOf is the account's windows as the session's snapshot keeps them:
// whole percent, the reset as a time. A window without a kind or outside
// 0–100 percent is not one; none off a subscription.
func limitsOf(raw json.RawMessage) []session.RateLimit {
	var windows []struct {
		Kind        string  `json:"kind"`
		PercentUsed float64 `json:"percentUsed"`
		ResetsAt    string  `json:"resetsAt"`
	}
	if json.Unmarshal(raw, &windows) != nil {
		return nil
	}
	var limits []session.RateLimit
	for _, window := range windows {
		percent := int(math.Round(window.PercentUsed))
		if window.Kind == "" || percent < 0 || percent > 100 {
			continue
		}
		limit := session.RateLimit{Kind: window.Kind, Percent: percent}
		if at, err := time.Parse(time.RFC3339Nano, window.ResetsAt); err == nil {
			at = at.Truncate(time.Second)
			limit.ResetsAt = &at
		}
		limits = append(limits, limit)
	}
	return limits
}

// effortOf is a request's effort, a level or an integer budget.
func effortOf(raw json.RawMessage) string {
	var effort any
	if json.Unmarshal(raw, &effort) != nil {
		return ""
	}
	switch effort := effort.(type) {
	case string:
		return effort
	case float64:
		return strconv.FormatFloat(effort, 'f', -1, 64)
	}
	return ""
}

// cacheOf is a main-loop response's usage as the API reported it, the cache's
// only source; nil for none or one with no input.
func cacheOf(raw json.RawMessage) *session.Cache {
	var usage struct {
		Read  *int64 `json:"cache_read_input_tokens"`
		Write *int64 `json:"cache_creation_input_tokens"`
		Input *int64 `json:"input_tokens"`
	}
	if json.Unmarshal(raw, &usage) != nil {
		return nil
	}
	count := func(value *int64) int64 {
		if value == nil {
			return 0
		}
		return *value
	}
	return session.NewCache(count(usage.Read), count(usage.Write), count(usage.Input))
}
