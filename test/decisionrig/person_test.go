//go:build rewaketest

package decisionrig

import (
	"path/filepath"
	"testing"
)

// The person classifier on real processes (docs/v2/stage3-decision.md#authority):
// a command a session runs is inside it whatever it does to its own
// environment or its process group, and the process that places it there is
// the session's own — the command itself or the harness the wrapper started
// with the session's variable — never one this test inherited from wherever
// it runs. One ordinary child and one detached child with a clean environment
// exercise both ways the process tree identifies the session.
func TestPersonDecisionOnRealProcesses(t *testing.T) {
	w := newWorld(t)
	web := w.launch("web", "--write", "")
	root := filepath.Join(w.root, "v2")
	for _, c := range []struct {
		name   string
		change func(*job)
		// harness says the session's harness places it, not the command.
		harness bool
	}{
		{"the session's command", nil, false},
		{"the session's command in a group of its own with a clean environment", func(j *job) { j.Detach, j.Clean = detachSetsid, true }, true},
	} {
		j := job{Mode: modePerson, Root: root}
		if c.change != nil {
			c.change(&j)
		}
		got := web.do(terminalHalf, command{Op: opAsk, Job: j})
		want := got.PID
		if c.harness {
			want = web.harness
		}
		if got.Verdict != "below" || got.Below != want {
			t.Errorf("%s: classified %+v, want below %d", c.name, got, want)
		}
	}
}
