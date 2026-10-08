package cli

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/codex/gateway"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

type integratedReservation struct {
	*gateway.Reservation
	ctx context.Context
}

func (r integratedReservation) Deliver(_ context.Context, m inbox.Message) inbox.Result {
	notice := gateway.MailboxNotice{Notice: harness.Notice(m)}
	members := m.Batch
	if len(members) == 0 {
		members = []inbox.Message{m}
	}
	for _, member := range members {
		notice.Members = append(notice.Members, gateway.MailboxMember{ID: member.ID, From: member.From, FromEpoch: member.FromEpoch, To: member.To, ToEpoch: member.ToEpoch})
	}
	_, err := r.Reservation.Deliver(r.ctx, m.ID, notice, nil)
	if err != nil {
		return inbox.Result{State: inbox.Failed, Detail: err.Error()}
	}
	return inbox.Result{State: inbox.Delivered, Via: "app-server"}
}

func waitIntegration(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("integration condition did not settle")
}

func TestGatewayMailboxReservationAndDurableReports(t *testing.T) {
	for _, mode := range []string{"A", "B", "side"} {
		t.Run(mode, func(t *testing.T) {
			completionThread := mode
			if mode == "side" {
				completionThread = "A"
			}
			dir := liveSession(t, "api")
			sender := otherRun(t, dir, "web")
			self, err := registry.Lookup(dir, "api")
			if err != nil {
				t.Fatal(err)
			}
			completed := make(chan gateway.Completion, 4)
			g, ui, native := gatewayWireFixture(t, self.Epoch(), func(c gateway.Completion) { completed <- c })
			wireExchange(t, ui, native, 1, "thread/start", map[string]any{"threadSource": "user", "runtimeWorkspaceRoots": []string{}}, map[string]any{"thread": map[string]any{"id": "A", "canAcceptDirectInput": true}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			served := make(chan struct{})
			server := inbox.Server{Dir: dir, Name: self.Name, Epoch: self.Epoch(), Reserve: func(ctx context.Context, _ inbox.Message) (inbox.Reservation, error) {
				r, err := g.Reserve(ctx)
				return integratedReservation{r, ctx}, err
			}}
			go func() { defer close(served); server.Serve(ctx) }()
			t.Cleanup(func() { cancel(); <-served })
			task := inbox.Message{ID: inbox.NewID(), From: sender.Name, FromEpoch: sender.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Question, Text: "work", CreatedAt: time.Now()}
			if err := inbox.Put(dir, task); err != nil {
				t.Fatal(err)
			}
			request, err := native.read()
			if err != nil {
				t.Fatal(err)
			}
			var params struct {
				Thread string `json:"threadId"`
			}
			_ = json.Unmarshal(request["params"], &params)
			if params.Thread != "A" {
				t.Fatal("wrong reserved destination")
			}
			if err := state.WithMailboxLock(context.Background(), dir, self.Name, func() error {
				unread, err := inbox.PeekUnread(dir, self.Name, self.Epoch())
				if err != nil {
					return err
				}
				if len(unread) != 1 || unread[0].ID != task.ID {
					t.Fatal("notice preceded readability")
				}
				return inbox.MarkRead(dir, self.Name, self.Epoch(), unread[0], true)
			}); err != nil {
				t.Fatal(err)
			}
			if err := native.write(map[string]any{"id": request["id"], "result": map[string]any{"turn": map[string]string{"id": "work"}}}); err != nil {
				t.Fatal(err)
			}
			wireNoticeDisplay(t, ui, "A", "work")
			waitIntegration(t, func() bool {
				status, ok, _ := inbox.ReadStatus(dir, self.Name, task.ID)
				return ok && status.State == inbox.Read
			})
			if mode == "side" {
				testSideWhilePrimaryWorks(t, dir, self, sender, g, ui, native, completed)
			} else {
				wireExchange(t, ui, native, 2, "thread/resume", map[string]any{"threadId": "B", "config": map[string]any{}, "runtimeWorkspaceRoots": []string{}}, map[string]any{"thread": map[string]any{"id": "B", "canAcceptDirectInput": true}})
			}
			for _, event := range []map[string]any{
				{"method": "turn/started", "params": map[string]any{"threadId": completionThread, "turn": map[string]string{"id": "work"}}},
				{"method": "item/completed", "params": map[string]any{"threadId": completionThread, "turnId": "work", "item": map[string]string{"type": "agentMessage", "text": "final result"}}},
				{"method": "turn/completed", "params": map[string]any{"threadId": completionThread, "turn": map[string]string{"id": "work", "status": "completed"}}},
			} {
				if mode == "side" && event["method"] == "turn/started" {
					continue
				}
				if err := native.write(event); err != nil {
					t.Fatal(err)
				}
				if _, err := ui.read(); err != nil {
					t.Fatal(err)
				}
			}
			var outcome gateway.Completion
			select {
			case outcome = <-completed:
			case <-time.After(time.Second):
				t.Fatal("completion lost after selection change")
			}
			event := inbox.TurnEnd{Boundary: boundaryNow(t, dir, self), ID: outcome.ID, Text: outcome.Text}
			if err := completeTurn(dir, self, event, outcome.Thread); err != nil {
				t.Fatal(err)
			}
			if mode == "side" {
				before := g.Binding()
				wireExchange(t, ui, native, 6, "thread/read", map[string]string{"threadId": "A"}, map[string]any{"thread": map[string]any{"id": "A", "canAcceptDirectInput": true}})
				wireExchange(t, ui, native, 7, "turn/interrupt", map[string]string{"threadId": "side", "turnId": "side-work"}, map[string]any{})
				wireExchange(t, ui, native, 8, "thread/unsubscribe", map[string]string{"threadId": "side"}, map[string]string{"status": "unsubscribed"})
				if g.Binding() != before {
					t.Fatal("side close lost primary")
				}
			}
			// Retry through the actual receipt path must not replace the original report batch.
			event.Text = "changed retry payload"
			if err := completeTurn(dir, self, event, outcome.Thread); err != nil {
				t.Fatal(err)
			}
			files := finishedFor(t, dir, sender.Name)
			if len(files) != 1 {
				t.Fatalf("duplicate/missing durable report: %v", files)
			}
			report := reportObject(t, dir, sender.Name)
			if report["text"] != "final result" || report["toEpoch"] != sender.Epoch() {
				t.Fatal(report)
			}
			changed, _ := report["threadChanged"].(bool)
			if changed != (completionThread == "B") {
				t.Fatalf("threadChanged=%v for completion %s", changed, completionThread)
			}
			failedCtx, stopFailed := context.WithCancel(context.Background())
			defer stopFailed()
			failedDone := make(chan struct{})
			recipient := inbox.Server{Dir: dir, Name: sender.Name, Epoch: sender.Epoch(), Reserve: func(context.Context, inbox.Message) (inbox.Reservation, error) {
				return nil, errors.New("notice target unavailable")
			}}
			go func() { defer close(failedDone); recipient.Serve(failedCtx) }()
			t.Cleanup(func() { stopFailed(); <-failedDone })
			reportID, _ := report["id"].(string)
			waitIntegration(t, func() bool {
				status, ok, _ := inbox.ReadStatus(dir, sender.Name, reportID)
				return ok && status.State == inbox.Failed && status.ReportAvailable
			})
			unread, err := inbox.AvailableUnread(dir, sender.Name, sender.Epoch())
			if err != nil || len(unread) != 1 || unread[0].Text != "final result" {
				t.Fatalf("lost-notice report unreadable: %v %v", unread, err)
			}
		})
	}
}
