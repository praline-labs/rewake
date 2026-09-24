package codex

import (
	"context"
	"errors"
	"fmt"
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
