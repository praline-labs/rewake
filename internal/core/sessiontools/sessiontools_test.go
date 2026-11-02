// Package sessiontools holds the session tools every adapter offers, defined
// once in sessiontools.ts and copied into each by tools/protogen; it has no
// Go of its own but the run of the module's tests.
package sessiontools

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// The tools hold their contract, the same in every harness: each input's
// arguments or refusal, and the result texts (sessiontools.test.ts).
func TestTheSessionToolsKeepTheirContract(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, bun, "test", "./sessiontools.test.ts")
	// A session running the tests exports rewake's variables; the tools'
	// tests get none of them.
	command.Env = slices.DeleteFunc(os.Environ(), func(entry string) bool {
		return strings.HasPrefix(entry, "REWAKE_")
	})
	out, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(out), " 0 fail") {
		t.Fatalf("the session tools' tests: %v\n%s", err, out)
	}
}
