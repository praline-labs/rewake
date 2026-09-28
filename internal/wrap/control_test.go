package wrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// steeredHarness serves control requests, and notes what its launch was
// given and what it found there.
type steeredHarness struct {
	*fakeHarness
	controlDir string
	mode       os.FileMode
}

func (s *steeredHarness) CompactFocus() bool     { return true }
func (s *steeredHarness) InterruptTrace() string { return "" }

func (s *steeredHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	s.controlDir = request.ControlDir
	if info, err := os.Stat(request.ControlDir); err == nil {
		s.mode = info.Mode().Perm()
	}
	return s.fakeHarness.Launch(request)
}

// A harness that serves control requests gets its run's directory, private,
// before it starts, and the directory goes with the session.
func TestTheControlDirectoryLivesAsLongAsTheSession(t *testing.T) {
	dir := stateDir(t)
	steered := &steeredHarness{fakeHarness: &fakeHarness{script: "exit 0"}}
	if _, err := Run(context.Background(), Request{Harness: steered, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if steered.controlDir == "" || steered.mode != 0o700 {
		t.Fatalf("launched with %q, mode %v", steered.controlDir, steered.mode)
	}
	if _, err := os.Stat(steered.controlDir); !os.IsNotExist(err) {
		t.Fatalf("the control directory outlived the session: %v", err)
	}
}

// A harness that does not serve them gets none: a request to it is refused
// before anything is written, and a directory would only say "not answering".
func TestAHarnessThatServesNoRequestsGetsNoDirectory(t *testing.T) {
	dir := stateDir(t)
	var given string
	fake := &launchSpy{fakeHarness: &fakeHarness{script: "exit 0"}, given: &given}
	if _, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if given != "" {
		t.Fatalf("launched with a control directory %q", given)
	}
	if entries, _ := os.ReadDir(filepath.Join(dir, "control")); len(entries) != 0 {
		t.Fatalf("made %v", entries)
	}
}

type launchSpy struct {
	*fakeHarness
	given *string
}

func (l *launchSpy) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	*l.given = request.ControlDir
	return l.fakeHarness.Launch(request)
}
