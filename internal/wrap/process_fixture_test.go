package wrap

import (
	"os"
	"os/exec"
	"regexp"
	"testing"
)

// A process reader fixture must not share globals with lifecycle readers from
// other tests. The child runs the same assertions and the same race-enabled binary.
func runProcessFixture(t *testing.T) bool {
	t.Helper()
	const marker = "REWAKE_PROCESS_FIXTURE_TEST"
	if os.Getenv(marker) == t.Name() {
		return false
	}
	child := exec.Command(os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.timeout=10s", "-test.v")
	child.Env = append(os.Environ(), marker+"="+t.Name())
	output, err := child.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated process fixture: %v\n%s", err, output)
	}
	t.Logf("isolated process fixture:\n%s", output)
	return true
}
