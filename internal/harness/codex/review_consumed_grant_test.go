package codex

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func TestReviewConsumedGrantCannotAuthorizeRemainingNotice(t *testing.T) {
	dir := t.TempDir()
	repo := gitRepository(t)
	task := inbox.Message{ID: inbox.NewID(), From: "main", FromEpoch: "main-run", To: "receiver", ToEpoch: "epoch", Kind: inbox.Task, GrantGit: true, Text: "grant-task", CreatedAt: time.Now()}
	note := inbox.Message{ID: inbox.NewID(), From: "peer", FromEpoch: "peer-run", To: "receiver", ToEpoch: "epoch", Kind: inbox.Note, Text: "remaining-note", CreatedAt: time.Now()}
	consumed := false
	read := gitReadFixture{thread: gitThreadFixture(repo, []string{repo}, "idle"), beforeReply: func() {
		err := state.WithMailboxLock(context.Background(), dir, "receiver", func() error {
			if err := inbox.MarkRead(dir, "receiver", "epoch", task, true); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Error(err)
		}
		consumed = true
	}}
	backend, captured := gitDeliveryFixture(t, role.Write, read)
	for _, m := range []inbox.Message{task, note} {
		if err := inbox.Put(dir, m); err != nil {
			t.Fatal(err)
		}
	}
	s := &inbox.Server{Dir: dir, Name: "receiver", Epoch: "epoch", Reserve: backend.Reserve, CheckGrant: confirmedGrant}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); s.Serve(ctx) }()
	defer func() { cancel(); <-done }()
	select {
	case params := <-captured:
		var id string
		_ = json.Unmarshal(params["clientUserMessageId"], &id)
		if id != note.ID || strings.Contains(string(params["toolOutput"]), "grant-task") {
			t.Fatal("consumed task entered dispatch")
		}
		if _, ok := params["runtimeWorkspaceRoots"]; ok {
			t.Fatal("consumed explicit grant widened note permissions")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("valid remaining note never dispatched")
	}
	cancel()
	<-done
	if !consumed {
		t.Fatal("did not consume during metadata preparation")
	}
	st, ok := inbox.ReadStatus(dir, "receiver", task.ID)
	if !ok || st.State != inbox.Read {
		t.Fatalf("lost actual read: %+v", st)
	}
	if len(inbox.Waiters(dir, "receiver", "epoch")) != 1 {
		t.Fatal("lost consumed task obligation")
	}
	select {
	case <-captured:
		t.Fatal("duplicate native work")
	default:
	}
}
