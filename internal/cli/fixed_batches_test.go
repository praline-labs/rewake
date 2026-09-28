package cli

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
)

func TestFixedTwoThenFourThroughPeekAndSelectedReads(t *testing.T) {
	for _, mode := range []string{"peek", "selected", "read-all"} {
		t.Run(mode, func(t *testing.T) {
			dir, self, peer := stateCaller(t, "write")
			put := func(kind inbox.Kind) inbox.Message {
				t.Helper()
				m := inbox.Message{ID: inbox.NewID(), From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: kind, Text: "batch content", CreatedAt: time.Now()}
				if err := inbox.Put(dir, m); err != nil {
					t.Fatal(err)
				}
				return m
			}
			first := []inbox.Message{put(inbox.Note), put(inbox.Note)}
			signals := make(chan inbox.Message, 4)
			s := &inbox.Server{Dir: dir, Name: self.Name, Epoch: self.Epoch(), Deliver: func(_ context.Context, m inbox.Message) inbox.Result {
				signals <- m
				return inbox.Result{State: inbox.Delivered}
			}}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); s.Serve(ctx) }()
			defer func() { cancel(); <-done }()
			wait := func(messages []inbox.Message, want inbox.State) {
				t.Helper()
				end := time.NewTimer(3 * time.Second)
				defer end.Stop()
				tick := time.NewTicker(10 * time.Millisecond)
				defer tick.Stop()
				for {
					all := true
					for _, m := range messages {
						st, ok := inbox.ReadStatus(dir, self.Name, m.ID)
						all = all && ok && st.State == want
					}
					if all {
						return
					}
					select {
					case <-tick.C:
					case <-end.C:
						t.Fatalf("messages not %s", want)
					}
				}
			}
			signal := func(want []inbox.Message) {
				t.Helper()
				select {
				case m := <-signals:
					if len(m.Batch) != len(want) || m.Unread != len(want) {
						t.Fatal("wrong count")
					}
					for i, member := range m.Batch {
						if member.ID != want[i].ID {
							t.Fatal("wrong fixed membership")
						}
					}
				case <-time.After(3 * time.Second):
					t.Fatal("missing signal")
				}
			}
			wait(first, inbox.Delivered)
			signal(first)
			if code := Run([]string{"inbox", "--peek"}, brokenWriter{}, brokenWriter{}); code == ExitOK {
				t.Fatal("failed peek reported success")
			}
			args := []string{"inbox", "--peek", "--json"}
			if mode == "selected" {
				args = []string{"inbox", "--message", first[0].ID, "--json"}
			}
			if mode == "read-all" {
				args = []string{"inbox", "--json"}
			}
			code, out, stderr := run(args...)
			if code != ExitOK {
				t.Fatal(stderr)
			}
			var view struct {
				Messages []struct {
					ID string `json:"id"`
				} `json:"messages"`
			}
			if json.Unmarshal([]byte(out), &view) != nil {
				t.Fatal(out)
			}
			for _, m := range view.Messages {
				if m.ID != first[0].ID && m.ID != first[1].ID {
					t.Fatal("overview acknowledged unannounced members")
				}
			}
			if len(inbox.Waiters(dir, self.Name, self.Epoch())) != 0 {
				t.Fatal("overview created task obligations")
			}
			later := []inbox.Message{put(inbox.Task), put(inbox.Question), put(inbox.Note), put(inbox.Finished)}
			wait(later, inbox.Delivered)
			signal(later)
			expected := 6
			if mode == "selected" {
				expected = 5
			}
			if mode == "read-all" {
				expected = 4
			}
			if list, _ := inbox.PeekUnread(dir, self.Name, self.Epoch()); len(list) != expected {
				t.Fatal("old unread lost or next members inflated")
			}
			if code, _, stderr := run("inbox", "--message", later[0].ID); code != ExitOK {
				t.Fatal(stderr)
			}
			if len(inbox.Waiters(dir, self.Name, self.Epoch())) != 1 {
				t.Fatal("actual selected task lost obligation")
			}
		})
	}
}
