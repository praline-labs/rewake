package claude

import (
	"fmt"
	"strconv"
	"strings"
)

// autocompactFlag sets the window auto-compaction keeps a conversation to.
// 2.1.302 takes "auto" or 100k–1M tokens, and reads the leading number of
// anything else ("300K (note)" is 300k), so rewake checks the form itself: a
// value it passed on would otherwise mean what Claude Code guessed.
const autocompactFlag = "--autocompact"

// defaultWindow is the window a worker's launch keeps to when nothing nearer names one:
// a typed --autocompact, a template's launch.context, REWAKE_CLAUDE_CONTEXT.
// The owner's decision (October 27, 2026), for workers only, not main; "auto" in any of them opts out to
// the model's own window.
const defaultWindow = "272k"

// Bounds of --autocompact, from its refusal on 2.1.302.
const (
	minWindow = 100_000
	maxWindow = 1_000_000
)

func (claudeHarness) LaunchSettings(model, effort, context string) ([]string, error) {
	var args []string
	if model != "" {
		args = append(args, modelFlag, model)
	}
	if effort != "" {
		args = append(args, effortFlag, effort)
	}
	if context != "" {
		if err := checkWindow(context); err != nil {
			return nil, err
		}
		args = append(args, autocompactFlag, context)
	}
	return args, nil
}

func (claudeHarness) AppliesContext() bool { return true }

// windowTokens is a window in tokens, 0 for auto, empty or anything
// parseWindow refuses.
func windowTokens(value string) int64 {
	tokens, err := parseWindow(value)
	if err != nil {
		return 0
	}
	return tokens
}

// checkWindow accepts auto, or a count of tokens written as 200000, 300k or
// 1M, between the bounds Claude Code allows.
func checkWindow(value string) error {
	_, err := parseWindow(value)
	return err
}

// parseWindow is the one reading of a window: its tokens, 0 for auto, or the
// refusal.
func parseWindow(value string) (int64, error) {
	if value == "auto" {
		return 0, nil
	}
	digits, scale := value, 1
	switch {
	case strings.HasSuffix(value, "k"), strings.HasSuffix(value, "K"):
		digits, scale = value[:len(value)-1], 1_000
	case strings.HasSuffix(value, "m"), strings.HasSuffix(value, "M"):
		digits, scale = value[:len(value)-1], 1_000_000
	}
	count, err := strconv.Atoi(digits)
	if err != nil || digits == "" || digits[0] == '+' || digits[0] == '-' || count > maxWindow || count*scale < minWindow || count*scale > maxWindow {
		return 0, fmt.Errorf("context is not a window Claude Code's %s takes: auto, or 100k to 1M tokens, such as 300k or 1M", autocompactFlag)
	}
	return int64(count * scale), nil
}
