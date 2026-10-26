package delivery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Only a real reservation earns a new delivery window. Its release is durable
// and recorded once, so retries and server restarts cannot renew it forever.
func (s *Server) answerExpired(message mail.Message, reserved bool) (bool, error) {
	path := filepath.Join(mail.RetentionPath(s.Dir, s.Name), message.ID)
	raw, err := state.ReadFile(path)
	var lifetime mail.AnswerLifetime
	if err == nil {
		if err := json.Unmarshal(raw, &lifetime); err != nil {
			return false, err
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if reserved && os.IsNotExist(err) {
		if err := state.EnsureSubdir(mail.RetentionPath(s.Dir, s.Name)); err != nil {
			return false, err
		}
		return false, state.WriteAtomic(path, []byte(`{"releasedAt":"0001-01-01T00:00:00Z"}`))
	}
	if reserved {
		return false, nil
	}
	if os.IsNotExist(err) {
		return s.expired(message), nil
	}
	if lifetime.ReleasedAt.IsZero() {
		lifetime.ReleasedAt = time.Now()
		encoded, err := json.Marshal(lifetime)
		if err != nil {
			return false, err
		}
		if err := state.WriteAtomic(path, encoded); err != nil {
			return false, err
		}
		unread := filepath.Join(state.UnreadPath(s.Dir, s.Name), message.ID+".json")
		_ = state.Touch(unread, lifetime.ReleasedAt)
	}
	return s.expired(mail.Message{CreatedAt: lifetime.ReleasedAt}), nil
}

// sweepFinished removes the messages and statuses that have been answered long
// enough ago that nobody is coming back for them.
//
// It retires the letters published to the mailbox, the proof that they
// landed, so it removes only under the mailbox lock, never in the unlocked
// fallback the server's other work takes, and only while its own run is the
// live run of the name, read inside that section; a server of a run that has
// ended would otherwise remove the live run's letters
// (docs/rules/effects.md#retiring-a-letter).
func (s *Server) sweepFinished() {
	ctx := s.lockContext
	if ctx == nil {
		ctx = context.Background()
	}
	_ = state.WithMailboxLock(ctx, s.Dir, s.Name, func() error {
		if !s.liveRun() {
			return nil
		}
		mail.SweepFinishedLocked(s.Dir, s.Name, s.Epoch)
		return nil
	})
}

// liveRun says whether this server's run is the live run of its name, read
// without the name lock's cleanup, which a holder of a mailbox lock must not
// enter.
func (s *Server) liveRun() bool {
	current, err := registry.LookupReadOnly(s.Dir, s.Name)
	return err == nil && s.Epoch != "" && current.Epoch() == s.Epoch
}
