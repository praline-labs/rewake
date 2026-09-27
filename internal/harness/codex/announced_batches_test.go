package codex

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

func TestReadyRecipientAnnouncesNextFourWithoutOverviewOrTerminal(t *testing.T) {
	for _, status := range []string{"idle", "active"} {
		for _, granted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/grant=%v", status, granted), func(t *testing.T) {
				peers := make(chan net.Conn, 1)
				var reads []gitReadFixture
				if granted {
					// An active thread keeps the grant waiting, read again on
					// each retry.
					repo := gitRepository(t)
					for range 8 {
						reads = append(reads, gitReadFixture{thread: gitThreadFixture(repo, []string{repo}, status)})
					}
				}
				backend, captured := gitDeliveryFixtureMode(t, role.Write, status, true, nil, func(c net.Conn) { peers <- c }, reads...)
				native := <-peers
				dir := t.TempDir()
				s := &inbox.Server{Dir: dir, Name: "receiver", Epoch: "epoch", Reserve: backend.Reserve, CheckGrant: confirmedGrant}
				put := func(kind inbox.Kind, grant bool) inbox.Message {
					t.Helper()
					m := inbox.Message{ID: inbox.NewID(), From: "main", FromEpoch: "main-epoch", To: s.Name, ToEpoch: s.Epoch, Kind: kind, GrantGit: grant, Text: "current preview", CreatedAt: time.Now()}
					if err := inbox.Put(dir, m); err != nil {
						t.Fatal(err)
					}
					return m
				}
				first := []inbox.Message{put(inbox.Note, false), put(inbox.Note, false)}
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan struct{})
				go func() { defer close(done); s.Serve(ctx) }()
				defer func() { cancel(); <-done }()
				waitState := func(messages []inbox.Message, want inbox.State) {
					t.Helper()
					deadline := time.NewTimer(3 * time.Second)
					defer deadline.Stop()
					tick := time.NewTicker(10 * time.Millisecond)
					defer tick.Stop()
					for {
						all := true
						for _, m := range messages {
							st, ok := inbox.ReadStatus(dir, s.Name, m.ID)
							all = all && ok && st.State == want
						}
						if all {
							return
						}
						select {
						case <-tick.C:
						case <-deadline.C:
							t.Fatalf("members did not reach %s", want)
						}
					}
				}
				rpc := func(count int, grant bool) {
					t.Helper()
					select {
					case p := <-captured:
						notice := decodedMailbox(t, p)
						if len(notice.Members) != count || !strings.Contains(notice.Notice, fmt.Sprintf("%d new message", count)) {
							t.Fatalf("wrong fixed count: %+v", notice)
						}
						_, has := p["runtimeWorkspaceRoots"]
						if has != grant {
							t.Fatal("grant applied to wrong group")
						}
					case <-time.After(3 * time.Second):
						t.Fatal("missing wake")
					}
				}
				waitState(first, inbox.Delivered)
				rpc(2, false)
				// Keep the accepted native turn active. No completion or overview follows.
				serverMessage(native, map[string]any{"method": "turn/started", "params": map[string]any{"threadId": fixtureRoot, "turn": map[string]string{"id": "delivered-turn-1"}}})
				later := []inbox.Message{}
				for _, kind := range []inbox.Kind{inbox.Note, inbox.Note, inbox.Note, inbox.Note} {
					later = append(later, put(kind, false))
				}
				if granted { // Replace only the last queued fixture before admission.
					later[3].Kind, later[3].GrantGit = inbox.Task, true
					if err := inbox.Put(dir, later[3]); err != nil {
						t.Fatal(err)
					}
				}
				readable := 6
				switch {
				case !granted:
					waitState(later, inbox.Delivered)
					rpc(4, false)
				case status == "idle":
					// A grant goes alone, after the mail before it.
					waitState(later, inbox.Delivered)
					rpc(3, false)
					rpc(1, true)
				default:
					// An active thread takes the mail before it and keeps
					// the grant waiting, not readable yet.
					waitState(later[:3], inbox.Delivered)
					rpc(3, false)
					waitState(later[3:], inbox.Pending)
					readable = 5
				}
				if unread, _ := inbox.PeekUnread(dir, s.Name, s.Epoch); len(unread) != readable {
					t.Fatal("old unread or new batch lost")
				}
				// Only old unread remains; another servicing opportunity cannot replay it.
				select {
				case <-captured:
					t.Fatal("old unread generated a reminder")
				case <-time.After(1100 * time.Millisecond):
				}
				next := put(inbox.Note, false)
				waitState([]inbox.Message{next}, inbox.Delivered)
				rpc(1, false)
			})
		}
	}
}
