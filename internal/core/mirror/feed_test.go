package mirror

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
)

// The feed every adapter's harness side sends through holds its contract:
// its batches, markers and cut texts (feed.test.ts).
func TestTheFeedKeepsItsContract(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, bun, "test", "./feed.test.ts")
	// A session running the tests exports rewake's variables; the feed's
	// tests get none of them.
	command.Env = slices.DeleteFunc(os.Environ(), func(entry string) bool {
		return strings.HasPrefix(entry, "REWAKE_")
	})
	out, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(out), " 0 fail") {
		t.Fatalf("the feed's tests: %v\n%s", err, out)
	}
}
