package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The one test that runs the real thing. It is gated on the suite's own
// switch, for the suite's own reason: the five required checks stay cheap, and
// nothing here starts a harness unless somebody asked for it.
//
// What it is for: the records are one package now, so a renamed field is a
// compile error and needs no test. What a compiler cannot say is whether a
// real run round-trips — that the suite prints records this program reads, in
// the order and the volume a real run produces them, and that the summary of
// a green run stays inside its budget. That is what this proves.
const switchEnv = "REWAKE_WORKFLOW"

// suiteBudget bounds the child. The whole suite takes about a minute and a
// half today; this is a ceiling on a hang, not an estimate.
const suiteBudget = 8 * time.Minute

func TestARealSuiteRunIsSummarized(t *testing.T) {
	if os.Getenv(switchEnv) == "" {
		t.Skipf("end-to-end run skipped: set %s=1 to run the suite through the summarizer", switchEnv)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	into := t.TempDir()
	// The summarizer runs the engine itself, which is the form the
	// documentation gives, so this exercises the path a person uses rather
	// than a convenience of the test.
	command := exec.Command("go", "run", "./tools/checksummary", "--into", into,
		"--", "go", "test", "-count=1", "-timeout", suiteBudget.String(), "-json", "./test/workflow/...")
	command.Dir = root
	command.Env = append(os.Environ(), switchEnv+"=1",
		// The child must not inherit this session's identity: a case that
		// believed it was the session running it would read a live mailbox.
		"REWAKE_SESSION=", "REWAKE_EPOCH=", "REWAKE_DIR=", "REWAKE_ROOM=")
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("the suite did not come out green through the summarizer: %v\n%s", err, out)
	}
	rendered := string(out)
	if !strings.Contains(rendered, "scenarios, ") || !strings.Contains(rendered, " pass") {
		t.Errorf("the summary does not name what ran:\n%s", rendered)
	}
	if lines := strings.Count(strings.TrimSpace(rendered), "\n") + 1; lines > 12 {
		// The budget in docs/check-runner.md, checked rather than promised.
		t.Errorf("a green run printed %d lines, over the budget of twelve:\n%s", lines, rendered)
	}
	entries, err := os.ReadDir(into)
	if err != nil || len(entries) != 1 {
		t.Fatalf("the summary directory holds %v (%v)", entries, err)
	}
	raw, err := os.ReadFile(filepath.Join(into, entries[0].Name(), "summary.json"))
	if err != nil {
		t.Fatalf("reading the summary: %v", err)
	}
	// Every case of a real run has a name and an outcome this program knows.
	// A record it could not read would have failed the run already; this is
	// the other half — that what it read is not empty.
	for _, want := range []string{`"case": "task-report"`, `"outcome": "pass"`, `"observations"`, `"engineExit": 0`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the summary file does not carry %s", want)
		}
	}
}
