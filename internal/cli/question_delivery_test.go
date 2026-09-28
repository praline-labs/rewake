package cli

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

func TestAFastAnswerIsNotAnnouncedTwice(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(epochEnv, web.Epoch())
	previous := awaitStatus
	defer func() { awaitStatus = previous }()
	var notices atomic.Int32
	awaitStatus = func(_ string, _ string, id string, _ time.Duration, _ func() bool) (inbox.Status, bool) {
		// Delivery may finish before the caller polls its status. The receiver's
		// end-of-turn report can already be on its way in this interval.
		report := inbox.Message{ID: inbox.NewID(), From: "api", To: "web", ToEpoch: web.Epoch(), Kind: inbox.Finished, InReplyTo: []string{id}, Text: "fast result", CreatedAt: time.Now()}
		if err := inbox.Put(dir, report); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		server := inbox.Server{Dir: dir, Name: "web", Epoch: web.Epoch(), Deliver: func(context.Context, inbox.Message) inbox.Result {
			notices.Add(1)
			return inbox.Result{State: inbox.Delivered}
		}}
		go func() { server.Serve(ctx); close(done) }()
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			files, _ := os.ReadDir(filepath.Join(state.InboxPath(dir, "web"), "unread"))
			if len(files) > 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Cleanup(func() { cancel(); <-done })
		return inbox.Status{State: inbox.Delivered}, true
	}
	code, out, errOut := run("send", "api", "question", "--question", "--wait", "3")
	t.Logf("exit=%d notices=%d stdout=%q stderr=%q", code, notices.Load(), out, errOut)
	if code != 0 {
		t.Fatalf("answer failed: %s", errOut)
	}
	if notices.Load() != 0 {
		t.Error("answer announced to harness and printed by waiting send")
	}
}
