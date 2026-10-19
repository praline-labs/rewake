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

// checkWindow accepts auto, or a count of tokens written as 200000, 300k or
// 1M, between the bounds Claude Code allows.
func checkWindow(value string) error {
	if value == "auto" {
		return nil
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
		return fmt.Errorf("context is not a window Claude Code's %s takes: auto, or 100k to 1M tokens, such as 300k or 1M", autocompactFlag)
	}
	return nil
}
