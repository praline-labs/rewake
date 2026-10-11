package cli

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	mail "github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func TestAFastAnswerIsNotAnnouncedTwice(t *testing.T) {
	dir := liveSession(t, "api")
	web := otherRun(t, dir, "web")
	t.Setenv(state.SessionEnv, "web")
	t.Setenv(epochEnv, web.Epoch())
	previous := awaitStatus
	defer func() { awaitStatus = previous }()
	var notices atomic.Int32
	awaitStatus = func(_ string, _ string, id string, _ time.Duration, _ func() bool) (mail.Status, bool) {
		// Delivery may finish before the caller polls its status. The receiver's
		// end-of-turn report can already be on its way in this interval.
		report := mail.Message{ID: mail.NewID(), From: "api", To: "web", ToEpoch: web.Epoch(), Kind: mail.Finished, InReplyTo: []string{id}, Text: "fast result", CreatedAt: time.Now()}
		if err := mail.Put(dir, report); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		server := mail.Server{Dir: dir, Name: "web", Epoch: web.Epoch(), Deliver: func(context.Context, mail.Message) mail.Result {
			notices.Add(1)
			return mail.Result{State: mail.Delivered}
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
		return mail.Status{State: mail.Delivered}, true
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
