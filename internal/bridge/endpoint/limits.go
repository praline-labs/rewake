package endpoint

import (
	"strconv"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The limits of one Claude Code call (docs/mail-bridge-launch.md#claude-code):
// the hook sends the raw MCP_TOOL_TIMEOUT and MAX_MCP_OUTPUT_TOKENS of its own
// environment with each observation, since which value the harness applies
// to a call is gate G8, not the launch's environment.

// HookLimits are the two settings as a hook saw them; nil is unset.
type HookLimits struct {
	Timeout *string `json:"timeout,omitempty"`
	Output  *string `json:"output,omitempty"`
}

// LimitsFrom reads the limits from an environment.
func LimitsFrom(lookup func(string) (string, bool)) HookLimits {
	var limits HookLimits
	if value, set := lookup("MCP_TOOL_TIMEOUT"); set {
		limits.Timeout = &value
	}
	if value, set := lookup("MAX_MCP_OUTPUT_TOKENS"); set {
		limits.Output = &value
	}
	return limits
}

// minTimeout is the least timeout that leaves a working deadline: 7000 ms
// less the 2 s margin is the 5 s a call needs.
const minTimeout = 7000

// callSpan is one call's deadline from the timeout its hook saw, or why it
// is refused before its ticket. Unset is the harness's default, which
// leaves the whole span. Only decimal digits count, and base 10 takes
// nothing else: a sign, a fraction, other text or a number past 64 bits
// names the setting and refuses that call alone, which is no evidence about
// the transport.
func callSpan(limits *HookLimits) (time.Duration, string) {
	if limits.Timeout == nil {
		return span, ""
	}
	value := *limits.Timeout
	millis, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, "MCP_TOOL_TIMEOUT is not a whole number of milliseconds, so this call runs nothing; set it to 7000 or more, or use the same words in the shell"
	}
	if millis < minTimeout {
		return 0, "MCP_TOOL_TIMEOUT=" + value + " leaves under five seconds for a tool call, so this call runs nothing; raise it to 7000 or more, or use the same words in the shell"
	}
	if millis >= uint64((span+timeoutLe)/time.Millisecond) {
		return span, ""
	}
	return time.Duration(millis)*time.Millisecond - timeoutLe, ""
}

// acknowledges says whether a call may acknowledge a read on its limits:
// the hooks' two snapshots agree, and the output limit is the default probe
// 2 calibrated or a whole number at or above the bound L5 calibrated for the
// launch's version. Without a bound any value counts as lowered
// (docs/mail-bridge-version.md).
func acknowledges(pre, post *HookLimits, bound int64) bool {
	if pre == nil || post == nil {
		return false
	}
	return same(pre.Timeout, post.Timeout) && same(pre.Output, post.Output) && outputWhole(pre.Output, bound)
}

// outputWhole says whether an output limit keeps a read whole: unset, or
// decimal digits naming at least the bound.
func outputWhole(output *string, bound int64) bool {
	if output == nil {
		return true
	}
	if bound <= 0 {
		return false
	}
	value, err := strconv.ParseUint(*output, 10, 64)
	return err == nil && value >= uint64(bound)
}

func same(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// hookPath says this run's calls are reported by hooks, which name a call's
// turn from its prompt and send the limits it runs under; on every other
// transport the binding names the turn.
func (e *Endpoint) hookPath() bool { return e.cfg.Transport == bridge.ClaudeTransport }

// deadline is how long a call's ticket lasts: on the hook path by the limits
// its hook saw, when it sent them, else by the launch's.
func (e *Endpoint) deadline(seen observation) (time.Duration, string) {
	if e.hookPath() && seen.limits != nil {
		return callSpan(seen.limits)
	}
	return e.cfg.Span, e.cfg.Refusal
}

// limitsAllow says whether a Claude Code call's limits let its result
// acknowledge a read: only once G8 closed, and only when its two snapshots
// agree on the default output limit. A hook of a test, which sends none, is
// taken as it always was.
func (e *Endpoint) limitsAllow(id string, post *HookLimits) bool {
	c := e.calls
	c.mu.Lock()
	entry := c.byCall[id]
	var pre *HookLimits
	if entry != nil && entry.observed != nil {
		pre = entry.observed.limits
	}
	c.mu.Unlock()
	if pre == nil && post == nil {
		return true
	}
	return e.cfg.LimitsProven && acknowledges(pre, post, e.cfg.OutputBound)
}
