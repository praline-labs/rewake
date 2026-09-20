package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDeliveryKeepsNativeTerminalOutcomesBeforeAndAfterACK(t *testing.T) {
	for _, tc := range []struct{ status, outcome, preceding string }{
		{"completed", "finished", "idle"}, {"failed", "error", "systemError"}, {"interrupted", "stopped", "idle"},
	} {
		for _, early := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/before-ack=%v", tc.status, early), func(t *testing.T) {
				g, ui, peers, _ := setup(t)
				native := <-peers
				defer func() { _ = native.conn.Close() }()
				bindUI(t, g, ui, native)
				outcomes := make(chan Completion, 4)
				g.mu.Lock()
				g.cfg.Complete = func(v Completion) { outcomes <- v }
				g.mu.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				r, err := g.Reserve(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				done := make(chan error, 1)
				go func() { _, err := r.Deliver(ctx, "notice", MailboxNotice{Notice: "new mail"}, nil); done <- err }()
				request := metadata(t, string(readWithin(t, native)))
				terminal := func() {
					write(t, native, []byte(fmt.Sprintf(`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":%q}}}`, tc.preceding)))
					_ = readWithin(t, ui)
					write(t, native, []byte(fmt.Sprintf(`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"accepted","status":%q}}}`, tc.status)))
					_ = readWithin(t, ui)
				}
				if early {
					terminal()
				}
				write(t, native, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"accepted"}}}`, request.idText)))
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				r.Close()
				if !early {
					terminal()
				}
				select {
				case outcome := <-outcomes:
					if outcome.ID != "A/accepted" || outcome.Kind != tc.outcome || outcome.Thread != "A" || !sameBinding(r.binding, Binding{Epoch: outcome.Epoch, Connection: outcome.Connection, Generation: outcome.Generation, Thread: outcome.Thread, Ready: true}) {
						t.Fatalf("lost admitted outcome: %+v", outcome)
					}
				case <-time.After(time.Second):
					t.Fatal("admitted task outcome was not published")
				}
				terminal()
				select {
				case outcome := <-outcomes:
					t.Fatalf("duplicate terminal republished outcome: %+v", outcome)
				default:
				}
			})
		}
	}
}
