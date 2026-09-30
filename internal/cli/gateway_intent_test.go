package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness/codex/gateway"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/sessionstate"
	"github.com/praline-labs/rewake/internal/state"
)

// The incident of September 29, 2026, through the inbox: the launch resumed X,
// the server refused it for another writer, the terminal started Y. A task
// sent then stays pending, naming both, and nothing of it becomes readable;
// the person's acceptance of Y lets it go.
func TestAFailedResumeKeepsTheTaskPendingUntilAccepted(t *testing.T) {
	dir := liveSession(t, "api")
	sender := otherRun(t, dir, "web")
	self, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	// As the launch does before the terminal starts (serverSession.holdMail).
	if err := sessionstate.HoldMail(dir, self.Name, self.Epoch(), "the launch asked to resume X"); err != nil {
		t.Fatal(err)
	}
	admit := func() error { return sessionstate.AdmitMail(dir, self.Name, self.Epoch()) }
	g, ui, native := gatewayWireFixtureConfig(t, gateway.Config{Epoch: self.Epoch(), Intent: gateway.LaunchIntent{Resume: true, Thread: "X"}, Name: self.Name, Admit: admit})
	if err := ui.write(map[string]any{"id": 1, "method": "thread/resume", "params": map[string]any{"threadId": "X", "config": map[string]any{}, "runtimeWorkspaceRoots": []string{}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := native.read(); err != nil {
		t.Fatal(err)
	}
	if err := native.write(map[string]any{"id": 1, "error": map[string]any{"code": -32600, "message": "thread X already has an active writer"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ui.read(); err != nil {
		t.Fatal(err)
	}
	if shown, err := ui.read(); err != nil || !strings.Contains(string(shown["params"]), "active writer") {
		t.Fatalf("the terminal was not told of the refusal: %s %v", shown["params"], err)
	}
	wireExchange(t, ui, native, 2, "thread/start", map[string]any{"threadSource": "user", "runtimeWorkspaceRoots": []string{}}, map[string]any{"thread": map[string]any{"id": "Y", "canAcceptDirectInput": true}})
	if shown, err := ui.read(); err != nil || !strings.Contains(string(shown["params"]), "rewake accept api Y") {
		t.Fatalf("the terminal was not told: %s %v", shown["params"], err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan struct{})
	server := inbox.Server{Dir: dir, Name: self.Name, Epoch: self.Epoch(), Reserve: func(ctx context.Context, _ inbox.Message) (inbox.Reservation, error) {
		r, err := g.Reserve(ctx)
		if errors.Is(err, gateway.ErrUnintended) {
			// As the Codex adapter maps it (reserveRefusal).
			return nil, fmt.Errorf("%w: %v", inbox.ErrNotYet, err)
		}
		return integratedReservation{r, ctx}, err
	}}
	go func() { defer close(served); server.Serve(ctx) }()
	t.Cleanup(func() { cancel(); <-served })
	task := inbox.Message{ID: inbox.NewID(), From: sender.Name, FromEpoch: sender.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Task, Text: "work", CreatedAt: time.Now()}
	if err := inbox.Put(dir, task); err != nil {
		t.Fatal(err)
	}
	waitIntegration(t, func() bool {
		status, ok := inbox.ReadStatus(dir, self.Name, task.ID)
		return ok && status.State == inbox.Pending && strings.Contains(status.Detail, "resume X") && strings.Contains(status.Detail, "selected Y")
	})
	unread, err := inbox.PeekUnread(dir, self.Name, self.Epoch())
	if err != nil || len(unread) != 0 {
		t.Fatalf("the task became readable in a conversation nobody chose: %v %v", unread, err)
	}
	if _, err := os.Stat(filepath.Join(state.InboxPath(dir, self.Name), "threads", task.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a delivery thread was recorded: %v", err)
	}
	// The worker in Y does not read the mail waiting for it either, though
	// no snapshot says so yet.
	t.Setenv(state.SessionEnv, self.Name)
	rawUnread(t, dir, self.Name, map[string]any{"from": sender.Name, "toEpoch": self.Epoch(), "text": "earlier task"})
	if code, out, errOut := run("inbox"); code != ExitFailed || strings.Contains(out, "earlier task") || !strings.Contains(errOut, "resume X") {
		t.Fatalf("the mail was read in a conversation nobody chose: exit %d, %q, %q", code, out, errOut)
	}
	if err := g.Accept("Y"); err != nil {
		t.Fatal(err)
	}
	if code, out, errOut := run("inbox"); code != ExitOK || !strings.Contains(out, "earlier task") {
		t.Fatalf("the accepted conversation could not read its mail: exit %d, %q, %q", code, out, errOut)
	}
	// The pending task goes at its next attempt, up to the retry interval
	// and a collection tick later: longer than one read waits.
	var request map[string]json.RawMessage
	for range 3 {
		if request, err = native.read(); !errors.Is(err, os.ErrDeadlineExceeded) {
			break
		}
	}
	if err != nil || !strings.Contains(string(request["params"]), `"threadId":"Y"`) {
		t.Fatalf("the accepted conversation did not get the notice: %s %v", request["params"], err)
	}
}
