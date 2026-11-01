package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// serveOnDisk plays the serving process of name, as far as its senders can
// see it, until the test ends. Each letter that lands in name's inbox gets the
// status answer gives it, written where a sender's wait reads it. A recall is
// skipped, since no sender waits on one. answer runs on a goroutine of its own
// for each letter and may block; its false writes nothing. Nothing is
// announced, and no readable copy is made: a withdrawal still finds the
// letter unseen.
func serveOnDisk(t *testing.T, dir, name string, answer func(id string) (mail.Status, bool)) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var running sync.WaitGroup
	running.Go(func() {
		seen := map[string]bool{}
		for ctx.Err() == nil {
			files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, name), "*.json"))
			for _, file := range files {
				id := strings.TrimSuffix(filepath.Base(file), ".json")
				if seen[id] || isRecall(file) {
					continue
				}
				seen[id] = true
				running.Go(func() {
					status, ok := answer(id)
					if !ok || ctx.Err() != nil {
						return
					}
					// Under the mailbox lock and never over a status written
					// already, as the server records an outcome: a withdrawal
					// that came first stays withdrawn.
					err := state.WithMailboxLock(ctx, dir, name, func() error {
						if _, known, err := mail.ReadStatus(dir, name, id); err != nil || known {
							return err
						}
						return mail.WriteStatus(dir, name, id, mail.Result{State: status.State, Via: status.Via, Detail: status.Detail})
					})
					if err != nil && ctx.Err() == nil {
						t.Errorf("the status of %s: %v", id, err)
					}
				})
			}
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Millisecond):
			}
		}
	})
	t.Cleanup(func() { cancel(); running.Wait() })
}

// delivered answers every letter delivered by the socket.
func delivered(string) (mail.Status, bool) {
	return mail.Status{State: mail.Delivered, Via: "socket"}, true
}

func isRecall(file string) bool {
	raw, err := os.ReadFile(file)
	var message mail.Message
	return err == nil && json.Unmarshal(raw, &message) == nil && message.Recall != nil
}
