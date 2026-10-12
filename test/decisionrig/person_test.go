//go:build rewakefixture

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
// it runs. The double fork with a clean environment is the accepted limit,
// unverified by design: what it classifies as depends on the process that
// adopts it, so it is logged, a real success apart from a refusal by the
// environment, and only an answer of no kind fails.
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
		{"the session's command with its variables unset", func(j *job) { j.Unset = []string{"REWAKE_SESSION", "REWAKE_EPOCH"} }, true},
		{"the session's command in a group of its own", func(j *job) { j.Detach = detachSetsid }, false},
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

	got := web.do(terminalHalf, command{Op: opAsk, Job: job{Mode: modePerson, Root: root, Detach: detachDouble, Clean: true}})
	switch got.Verdict {
	case "person":
		t.Logf("real success of the accepted limit: a double fork with a clean environment classifies as the person (pid %d)", got.PID)
	case "below":
		t.Logf("the process that adopted the double fork is inside a session here: below %d: %s", got.Below, got.Error)
	case "unreadable":
		t.Logf("refused by the environment: %s", got.Error)
	default:
		t.Errorf("the double fork classified as no kind: %+v", got)
	}
}
