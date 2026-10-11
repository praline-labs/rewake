package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// realReporting gives back each harness's own answer for the test.
func realReporting(t *testing.T) {
	t.Helper()
	saved := reportsTurns
	reportsTurns = harnessReportsTurns
	t.Cleanup(func() { reportsTurns = saved })
}

// unreported is a harness of the catalog whose contract reports no turn's
// end; the test has nothing to show where every one reports.
func unreported(t *testing.T) adapter.Harness {
	t.Helper()
	for _, h := range adapter.All() {
		if !harnessReportsTurns(h) {
			return h
		}
	}
	t.Skip("every harness of this build reports its turns")
	return nil
}

// A role that owes reports does not launch on a harness whose contract
// reports no turn's end, and says from when it will; nothing is claimed. Main
// owes none, and passes.
func TestAReportingRoleDoesNotLaunchOnAHarnessThatReportsNoTurns(t *testing.T) {
	realReporting(t)
	silent := unreported(t)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, dir)
	outsideAnySession(t)
	for _, flags := range [][]string{{silent.ID()}, {"--general", silent.ID()}, {"--write", silent.ID()}} {
		code, out, errOut := run(flags...)
		if code != ExitFailed || out != "" || !strings.Contains(errOut, "has no turn boundary in this rewake") ||
			!strings.Contains(errOut, "available on "+silent.Title()+" from stage 4") || !strings.Contains(errOut, "--main") {
			t.Errorf("%v: exit %d, %q, %q", flags, code, out, errOut)
		}
	}
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			t.Errorf("a refused launch left %s", path)
		}
		return nil
	})
	if !strings.Contains(firstLineOf(t, silent.ID()), "a general session owes a report") {
		t.Error("the refusal of a launch with no role flag does not name the default role")
	}
	if err := refuseReportingRole(silent, role.Main); err != nil {
		t.Fatalf("main was refused: %v", err)
	}
}

// A task or a question to a session on such a harness fails, and nothing is
// sent; a note still goes. The recipient is a main, the one role that launches
// there, so the question meets the harness's refusal before main's own: the
// role's advice to send a plain task would only be refused next. The sessions
// of these tests are recorded under an adapter that reports no turns in an
// untagged build; TestAQuestionToASilentSessionIsRefused keeps main's refusal
// on a harness that reports.
func TestATaskToASessionThatReportsNoTurnsFails(t *testing.T) {
	realReporting(t)
	unreported(t)
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	web.Role = role.Main.ID
	if err := registry.Update(dir, web); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, "api")
	for kind, args := range map[string][]string{
		"task":     {"send", "web", "run the suite", "--wait", "0"},
		"question": {"send", "web", "which port?", "--question", "--wait", "0"},
	} {
		code, _, errOut := run(args...)
		if code != ExitFailed || !strings.Contains(errOut, "web runs ") || !strings.Contains(errOut, ", which has no turn boundary") ||
			!strings.Contains(errOut, "a "+kind+" to it would never be answered; nothing was sent") {
			t.Errorf("%s: exit %d, %q", kind, code, errOut)
		}
	}
	if sent := finishedFor(t, dir, "web"); len(sent) != 0 {
		t.Fatalf("a refused send left %v", sent)
	}
	if code, _, errOut := run("send", "web", "merged", "--notify", "--wait", "0"); code != ExitPending {
		t.Fatalf("a note: exit %d, %q", code, errOut)
	}
	if sent := finishedFor(t, dir, "web"); len(sent) != 1 {
		t.Fatalf("the note left %v", sent)
	}
}

// firstLineOf is the first line a launch's refusal prints.
func firstLineOf(t *testing.T, argv ...string) string {
	t.Helper()
	_, _, errOut := run(argv...)
	line, _, _ := strings.Cut(errOut, "\n")
	return line
}
