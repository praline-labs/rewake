package wrap

import (
	"context"
	"errors"
	"testing"

	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/registry"
)

// The launch's version (docs/mail-bridge-version.md): a harness that reads
// it on every launch reads it once, before the claim, --no-mail-tool
// included; the read reaches the check and the plan, and a read that could
// not end refuses the launch before anything is claimed.

// readerHarness is the tool harness as one that reads its version at launch.
type readerHarness struct {
	*toolHarness
	version  harness.Version
	err      error
	reads    int
	asked    *harness.Version
	launched harness.Version
}

func (h *readerHarness) ID() string { return "codex" }

func (h *readerHarness) ReadLaunchVersion(string, []string, string) (harness.Version, error) {
	h.reads++
	return h.version, h.err
}

func (h *readerHarness) CheckMailTool(request harness.ToolCheckRequest) (harness.ToolDecision, error) {
	h.asked = request.Version
	return h.toolHarness.CheckMailTool(request)
}

func (h *readerHarness) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	h.launched = request.Version
	return h.toolHarness.Launch(request)
}

func TestTheLaunchReadsItsVersionOnceBeforeTheClaim(t *testing.T) {
	for _, noTool := range []bool{false, true} {
		dir := shortStateDir(t)
		h := &readerHarness{
			toolHarness: &toolHarness{fakeHarness: &fakeHarness{script: "exit 0"}, decision: harness.ToolDecision{Reason: "gate G2"}},
			version:     harness.Version{Value: "0.159.0"},
		}
		request := toolRequest(dir, h.toolHarness)
		request.Harness, request.NoMailTool = h, noTool
		if _, err := Run(context.Background(), request); err != nil {
			t.Fatal(err)
		}
		if h.reads != 1 || h.launched != h.version {
			t.Fatalf("--no-mail-tool %v: %d reads, the plan was given %+v", noTool, h.reads, h.launched)
		}
		switch {
		case noTool && (h.checked != 0 || h.asked != nil):
			t.Fatal("--no-mail-tool asked the check")
		case !noTool && (h.asked == nil || *h.asked != h.version):
			t.Fatalf("the check was given %v", h.asked)
		}
	}
}

func TestAVersionReadThatCannotEndRefusesBeforeTheClaim(t *testing.T) {
	dir := shortStateDir(t)
	refusal := &harness.CheckFailedError{Program: "codex", Label: "--version", Outcome: harness.OutcomeNotEnded}
	h := &readerHarness{toolHarness: &toolHarness{fakeHarness: &fakeHarness{script: "exit 0"}}, err: refusal}
	request := toolRequest(dir, h.toolHarness)
	request.Harness = h
	_, err := Run(context.Background(), request)
	var failed *harness.CheckFailedError
	if !errors.As(err, &failed) || h.checked != 0 || h.called {
		t.Fatalf("got %v, checked %d, launched %v", err, h.checked, h.called)
	}
	if sessions, _ := registry.ListReadOnly(dir); len(sessions) != 0 {
		t.Fatalf("a refused launch published %d sessions", len(sessions))
	}
}

// withdrawnBackend starts and says its start left the tool out.
type withdrawnBackend struct {
	harness.Backend
	reason string
}

func (withdrawnBackend) Start(context.Context, harness.CompletionHandler, func(string)) error {
	return nil
}
func (withdrawnBackend) Close()                  {}
func (b withdrawnBackend) ToolWithdrawn() string { return b.reason }

// A tool the start withdrew begins the record again as a run without it,
// with the reason; one the start kept leaves the record as chosen.
func TestAWithdrawnToolBeginsTheRecordWithoutIt(t *testing.T) {
	for _, reason := range []string{"harness version not confirmed", ""} {
		dir := stateDir(t)
		tool := &mailTool{keeper: newChannelKeeper(dir, "api", "1.2.b", "codex")}
		tool.keeper.begin(true, "")
		plan := harness.LaunchPlan{Backend: withdrawnBackend{reason: reason}}
		stop, err := startServing(context.Background(), Request{Dir: dir}, registry.Session{}, "api", "1.2.b", plan, tool)
		if err != nil {
			t.Fatal(err)
		}
		stop()
		record := tool.keeper.snapshot()
		if withdrawn := record.Tool == channel.ToolNone; withdrawn != (reason != "") || record.Reason != reason {
			t.Fatalf("withdrawn %q: %+v", reason, record)
		}
	}
}
