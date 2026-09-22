package workflow

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// selfCheckEnv selects the deliberately incomplete scenario below. It is a
// separate switch from REWAKE_WORKFLOW so that the fixture never runs as part
// of an ordinary suite run — it is designed to fail.
const selfCheckEnv = "REWAKE_WORKFLOW_SELFCHECK"

// selfCheckTimeout bounds the child. It has to build a binary of its own, so
// it is not instant, but it must never be able to hang the parent.
const selfCheckTimeout = 4 * time.Minute

// TestVerdictIsPublishedThroughTheRealPath re-runs this test binary as a child
// and asserts that a scenario which skips a required observation makes the run
// fail.
//
// The stand-in in classification_test.go proves the classifier; it cannot
// prove that a real scenario reaches it. Start registers the finaliser through
// t.Cleanup, and if that registration were ever dropped, every scenario would
// still look green: observations made, nobody reporting, no verdict published,
// no test noticing. Only a child process can observe that, because a scenario
// that correctly fails would otherwise fail the test watching it.
func TestVerdictIsPublishedThroughTheRealPath(t *testing.T) {
	if os.Getenv(selfCheckEnv) != "" {
		t.Skip("this process is the child; the fixture runs instead")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	// Bounded twice: the child gets its own -test.timeout so it fails with a
	// stack rather than hanging, and the context ends the process group if it
	// ignores that too.
	ctx, stop := context.WithTimeout(context.Background(), selfCheckTimeout)
	defer stop()
	child := exec.Command(executable,
		"-test.run=^TestIncompleteScenarioFixture$", "-test.v",
		"-test.timeout="+selfCheckTimeout.String())
	child.Env = append(os.Environ(),
		switchEnv+"=1",
		selfCheckEnv+"=1",
		// The child is meant to fail, and a failing case keeps its directory
		// as evidence. Pointing its temporary root inside this test's own
		// means the deliberate failure does not litter /tmp on every run.
		"TMPDIR="+t.TempDir(),
		// The child must not inherit this session's identity either.
		"REWAKE_SESSION=", "REWAKE_EPOCH=", "REWAKE_DIR=", "REWAKE_ROOM=",
		// Nor a harness version: the child is switched on, so a version
		// named here would send it to the registry and to docker from inside
		// the five ordinary checks, which start neither.
		codexVersionEnv+"=")
	// Bounded through the group machinery: the child builds a binary of its
	// own, so its descendants have to be ended with it rather than left to
	// hold the pipe open.
	out, err := outputBounded(ctx, "self-check child", child)
	if ctx.Err() != nil {
		t.Fatalf("the self-check child did not finish within %s:\n%s", selfCheckTimeout, out)
	}
	if err == nil {
		t.Fatalf("a scenario missing a required observation passed:\n%s", out)
	}
	if !strings.Contains(string(out), string(Incomplete)) {
		t.Errorf("the child failed, but not as incomplete:\n%s", out)
	}
	if !strings.Contains(string(out), "second observation") {
		t.Errorf("the failure does not name the observation that was never made:\n%s", out)
	}
}

// TestIncompleteScenarioFixture is the child. It declares two observations and
// makes one, so the run it belongs to must fail.
func TestIncompleteScenarioFixture(t *testing.T) {
	if os.Getenv(selfCheckEnv) == "" {
		t.Skip("fixture for TestVerdictIsPublishedThroughTheRealPath")
	}
	binary := enterScenario(t, "self-check-incomplete")
	c := Start(t, Spec{
		Name:         "self-check-incomplete",
		Observations: []string{"first observation", "second observation"},
	})
	Isolate(t, c, binary)
	c.Observed("first observation", "made")
	// "second observation" is never made: that is the whole point.
}
