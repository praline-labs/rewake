package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A process that outlives every case fails the run in the suite's TestMain,
// after the last case has published its record. The only account of it is
// the run record, so this runs the suite's package for real, with one leak
// planted, and reads what the summary says: the leftover's command line and
// that it carried no owner label, not a red engine with no case to blame.
func TestALeftoverProcessIsNamedInTheSummary(t *testing.T) {
	const marker = "planted-leak-for-checksummary"
	// The child inherits this environment. The session's identity is cleared
	// as the documented commands clear it, and the switch is off: the planted
	// test needs no scenario, and none should start.
	for _, name := range []string{"REWAKE_SESSION", "REWAKE_EPOCH", "REWAKE_DIR", "REWAKE_ROOM", switchEnv} {
		t.Setenv(name, "")
	}
	t.Setenv("REWAKE_WORKFLOW_PLANT_LEAK", marker)
	into := t.TempDir()
	out, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	code := run([]string{
		"--into", into, "--", "go", "test", "-count=1", "-json",
		"-run", "^TestAPlantedLeak$", "../../test/workflow/",
	}, out, out)
	rendered, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	if code != exitRed {
		t.Fatalf("a run that left a process exited %d, want %d:\n%s", code, exitRed, rendered)
	}
	for _, want := range []string{"run       FAIL  processes outlived their cases", marker, "no owner label"} {
		if !strings.Contains(string(rendered), want) {
			t.Errorf("the summary does not say %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(string(rendered), "with no case reporting a failure") {
		t.Errorf("the summary blames no one for a failure it can name:\n%s", rendered)
	}
	entries, err := os.ReadDir(into)
	if err != nil || len(entries) != 1 {
		t.Fatalf("the summary directory holds %v (%v)", entries, err)
	}
	raw, err := os.ReadFile(filepath.Join(into, entries[0].Name(), "summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), marker) {
		t.Errorf("the summary file does not name the leftover:\n%s", raw)
	}
}
