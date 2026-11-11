package workmode

import (
	"strings"
	"testing"
)

// One mode's text is the same for every session; what the launch's authorship
// policy adds is a qualification, not a second discipline. A delegating main
// is told so in every mode, an implementing main is told what still bounds it,
// and a worker is told neither.
func TestInstructionsCarryTheLaunchsAuthorshipPolicy(t *testing.T) {
	for _, name := range []string{Advisor, Build, Review, Process} {
		mode := BuiltIns()[name]
		delegating := Instructions(mode, true, false)
		if !strings.Contains(delegating, "delegate implementation; do not write product code yourself") {
			t.Errorf("%s: a delegating main is not told to delegate: %q", name, delegating)
		}
		implementing := Instructions(mode, true, true)
		if !strings.Contains(implementing, "implementation is permitted only when the task, the mode and the project's rules allow it") {
			t.Errorf("%s: an implementing main is not bounded: %q", name, implementing)
		}
		if !strings.Contains(implementing, "grants no additional permissions") {
			t.Errorf("%s: the policy claims a permission: %q", name, implementing)
		}
		if strings.Contains(implementing, "do not write product code yourself") {
			t.Errorf("%s: an implementing main is still forbidden: %q", name, implementing)
		}
		for _, text := range []string{implementing, delegating} {
			if !strings.Contains(text, mode.Text) || strings.Count(text, "Runtime work mode:") != 1 {
				t.Errorf("%s: not one whole section: %q", name, text)
			}
		}
		worker := Instructions(mode, false, true)
		if strings.Contains(worker, "do not write product code yourself") || strings.Contains(worker, "implementation is permitted") {
			t.Errorf("%s: a worker got main's qualification: %q", name, worker)
		}
	}
}
