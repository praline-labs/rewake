package codex

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/control"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/harness/codex/gateway"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func TestTheWrapperServesTheRunsControlDirectory(t *testing.T) {
	codexHome(t, "")
	plan, err := New().Launch(harness.LaunchRequest{Name: "writer", Dir: t.TempDir(), Socket: "/tmp/session.sock", Epoch: "1.2", ControlDir: "/tmp/control/writer.1.2"})
	if err != nil {
		t.Fatal(err)
	}
	if server := plan.Backend.(*serverSession); server.controlDir != "/tmp/control/writer.1.2" {
		t.Fatalf("control directory %q", server.controlDir)
	}
}

func TestSteerAnswersWhatItCannotCarryOut(t *testing.T) {
	server := &serverSession{}
	for _, tc := range []struct {
		request control.Request
		detail  string
	}{
		{control.Request{Action: "reboot"}, "unknown action reboot"},
		{control.Request{Action: control.Compact}, "the app-server is not connected yet"},
		{control.Request{Action: control.Interrupt}, "the app-server is not connected yet"},
	} {
		if answer := server.steer(context.Background(), tc.request); answer.Outcome != control.Failed || answer.Detail != tc.detail {
			t.Fatalf("%+v: %+v", tc.request, answer)
		}
	}
}

// A compaction withdrawn before it was taken is a final answer the wrapper
// gives without the gateway, and its outcome all the same; an interrupt has
// no letter to feed.
func TestAWithdrawnCompactionIsItsOutcome(t *testing.T) {
	server := &serverSession{gateway: gateway.New(gateway.Config{Upstream: filepath.Join(t.TempDir(), "app-server.sock"), Epoch: "test"})}
	t.Cleanup(server.gateway.Close)
	withdrawn := control.Answer{ID: "0123", Outcome: control.Refused, Reason: control.Withdrawn}
	server.withdrawn(control.Request{ID: "0123", Action: control.Compact, From: "lead"}, withdrawn)
	server.withdrawn(control.Request{ID: "4567", Action: control.Interrupt, From: "lead"}, control.Answer{ID: "4567", Outcome: control.Refused, Reason: control.Withdrawn})
	outcomes := server.gateway.SessionState().CompactionOutcomes
	if len(outcomes) != 1 || outcomes[0].Request != "0123" || outcomes[0].RequestedBy != "lead" ||
		outcomes[0].Outcome != control.Refused || outcomes[0].Reason != control.Withdrawn {
		t.Fatalf("outcomes %+v", outcomes)
	}
}

// A delivery refused while a compaction runs stays pending in the inbox and
// goes after it; any other refusal still fails it.
func TestAReservationRefusedForACompactionWaits(t *testing.T) {
	if err := reserveRefusal(fmt.Errorf("%w: deadline", gateway.ErrCompacting)); !errors.Is(err, inbox.ErrNotYet) {
		t.Fatalf("compacting: %v", err)
	}
	if err := reserveRefusal(errors.New("conversation changed while waiting for delivery admission")); errors.Is(err, inbox.ErrNotYet) {
		t.Fatalf("a changed conversation waits: %v", err)
	}
}
