package codex

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
)

func TestTerminalIdleOnlyWakesNewUnannouncedMembers(t *testing.T) {
	for _, status := range []string{"failed", "interrupted"} {
		t.Run(status, func(t *testing.T) {
			f, first := reviewBatches(t)
			f.emit("delivered-turn-1", status)
			serverMessage(f.native, map[string]any{"method": "thread/status/changed", "params": map[string]any{"threadId": fixtureRoot, "status": map[string]string{"type": "idle"}}})
			// Cross multiple service opportunities: terminal/idle is permission to send
			// new mail, not a reason to replay the old unread group.
			select {
			case <-f.captured:
				t.Fatal("terminal/idle replayed old work without new mail")
			case <-time.After(1300 * time.Millisecond):
			}
			var next []inbox.Message
			for _, kind := range []inbox.Kind{inbox.Task, inbox.Question, inbox.Note, inbox.Finished} {
				next = append(next, f.put(t, kind, "new-"+string(kind)))
			}
			f.notice(t, next)
			f.wait(t, next, inbox.Delivered)
			unread, err := inbox.PeekUnread(f.dir, f.server.Name, f.server.Epoch)
			if err != nil || len(unread) != 6 {
				t.Fatalf("unread=%d err=%v", len(unread), err)
			}
			for _, m := range first {
				st, ok, _ := inbox.ReadStatus(f.dir, m.To, m.ID)
				if !ok || st.State != inbox.Delivered {
					t.Fatal("old announced receipt changed")
				}
			}
			select {
			case <-f.captured:
				t.Fatal("new-mail release generated duplicate native work")
			default:
			}
		})
	}
}
