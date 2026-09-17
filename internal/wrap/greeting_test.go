package wrap

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

type greetingHarness struct{ fakeHarness }

func (h *greetingHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	plan, err := h.fakeHarness.Launch(request)
	plan.Greeting = request.Greeting
	return plan, err
}

func TestBootstrapIsMarkedBeforeTheHarnessStarts(t *testing.T) {
	dir := stateDir(t)
	path := filepath.Join(state.InboxPath(dir, "api"), "greeting")
	fake := &greetingHarness{fakeHarness{script: fmt.Sprintf(`test "$(cat %q)" = "$REWAKE_EPOCH"`, path)}}
	code, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api", Greeting: true})
	if err != nil || code != 0 {
		t.Fatalf("bootstrap was not present at launch: %d %v", code, err)
	}
}
