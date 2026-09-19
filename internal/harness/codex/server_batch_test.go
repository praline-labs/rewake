package codex

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

func TestMixedBatchSharesNativeACKAndUnionsTaskGrants(t *testing.T) {
	for _, part := range []role.Role{role.General, role.Write, role.Main} {
		for _, work := range []bool{false, true} {
			label := "reports"
			if work {
				label = "mixed"
			}
			t.Run(part.ID+"/"+label, func(t *testing.T) {
				repo := gitRepository(t)
				roots := []string{repo, t.TempDir()}
				grant := work && part.GitWrite
				var reads []gitReadFixture
				if grant {
					reads = []gitReadFixture{{thread: gitThreadFixture(repo, roots, "idle")}}
				}
				ack := make(chan struct{})
				var release sync.Once
				releaseACK := func() { release.Do(func() { close(ack) }) }
				defer releaseACK()
				backend, captured := gitDeliveryFixtureWithAck(t, part, func() { <-ack }, reads...)
				dir := t.TempDir()
				var members []inbox.Message
				kinds := []inbox.Kind{inbox.Finished, inbox.Note, inbox.Error, inbox.Stopped}
				if work {
					kinds = []inbox.Kind{inbox.Finished, inbox.Question, inbox.Note, inbox.Task}
				}
				for _, kind := range kinds {
					m := inbox.Message{ID: inbox.NewID(), From: "sender", FromEpoch: "sender-epoch", To: "receiver", ToEpoch: "epoch", Kind: kind, GrantGit: grant && (kind == inbox.Task || kind == inbox.Question), Text: "Brief " + string(kind) + "\nPRIVATE FULL BODY " + string(kind), CreatedAt: time.Now()}
					if err := inbox.Put(dir, m); err != nil {
						t.Fatal(err)
					}
					members = append(members, m)
				}
				var reservations atomic.Int32
				server := &inbox.Server{Dir: dir, Name: "receiver", Epoch: "epoch", Reserve: func(ctx context.Context, m inbox.Message) (inbox.Reservation, error) {
					reservations.Add(1)
					return backend.Reserve(ctx, m)
				}}
				ctx, cancel := context.WithCancel(context.Background())
				done := make(chan struct{})
				go func() { defer close(done); server.Serve(ctx) }()
				defer func() { releaseACK(); cancel(); <-done }()
				var params map[string]json.RawMessage
				select {
				case params = <-captured:
				case <-time.After(3 * time.Second):
					t.Fatal("group did not reach native transport")
				}
				raw, _ := json.Marshal(params)
				if !strings.Contains(string(raw), "↳") || strings.Contains(string(raw), "--peek") || strings.Contains(string(raw), "PRIVATE FULL BODY") {
					t.Fatal("group announcement lost compact preview or leaked instructions/body")
				}
				var id string
				_ = json.Unmarshal(params["clientUserMessageId"], &id)
				if !strings.HasPrefix(id, "group-") {
					t.Fatal("native request lost group identity")
				}
				var gotRoots []string
				_ = json.Unmarshal(params["runtimeWorkspaceRoots"], &gotRoots)
				if grant {
					if !slices.Equal(gotRoots, append(slices.Clone(roots), filepath.Join(repo, ".git"))) {
						t.Fatalf("mixed task grant replaced roots: %q", gotRoots)
					}
				} else if gotRoots != nil {
					t.Fatal("report-only/ordinary role gained roots")
				}
				for _, m := range members {
					if status, ok := inbox.ReadStatus(dir, m.To, m.ID); ok && status.State == inbox.Delivered {
						t.Fatal("member settled before shared native ACK")
					}
				}
				unread, err := inbox.PeekUnread(dir, "receiver", "epoch")
				if err != nil || len(unread) != 4 {
					t.Fatal("member unread publication did not precede native RPC")
				}
				releaseACK()
				deadline := time.NewTimer(2 * time.Second)
				defer deadline.Stop()
				ticker := time.NewTicker(10 * time.Millisecond)
				defer ticker.Stop()
				for {
					all := true
					for _, m := range members {
						status, ok := inbox.ReadStatus(dir, m.To, m.ID)
						all = all && ok && status.State == inbox.Delivered
					}
					if all {
						break
					}
					select {
					case <-ticker.C:
					case <-deadline.C:
						t.Fatal("shared ACK did not settle every member")
					}
				}
				cancel()
				<-done
				// The deferred wait sees the closed channel and is harmless.
				if reservations.Load() != 1 {
					t.Fatal("multiple native admission gates for one group")
				}
				select {
				case <-captured:
					t.Fatal("group injected more than one native request")
				default:
				}
			})
		}
	}
}
