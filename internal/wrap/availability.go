package wrap

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// One readiness worker per launch, and one discovery loop only for room main.
// Notification/storage errors never enter the child or mailbox lifecycle paths.
func startAvailability(parent context.Context, dir string, self registry.Session, ready <-chan struct{}, usable func() bool) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			return
		case <-ready:
		}
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for ctx.Err() == nil {
			if usable() && markMessagingReady(dir, self) == nil {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
		if ctx.Err() != nil || self.Role != role.Main.ID {
			return
		}
		ticker.Reset(time.Second)
		notices := newSessionNotices()
		for ctx.Err() == nil {
			if usable() {
				_ = notices.scan(ctx, dir, self)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func markMessagingReady(dir string, self registry.Session) error {
	current, err := registry.LookupReadOnly(dir, self.Name)
	if err != nil {
		return err
	}
	if current.Epoch() != self.Epoch() {
		return registry.ErrNotFound
	}
	if current.MessagingReadyAt != nil {
		return nil
	}
	now := time.Now()
	current.MessagingReadyAt = &now
	return registry.Update(dir, current)
}

func availabilityMessage(self, peer registry.Session) inbox.Message {
	sum := sha256.Sum256([]byte("availability\x00" + self.Name + "\x00" + self.Epoch() + "\x00" + peer.Name + "\x00" + peer.Epoch()))
	timestamp := max(self.StartedAt.UnixNano(), peer.StartedAt.UnixNano())
	id := fmt.Sprintf("%019d-%x", timestamp, sum[:12])
	existing := peer.MessagingReadyAt != nil && peer.MessagingReadyAt.Before(self.StartedAt)
	label := "Session available."
	if existing {
		label = "Session already available in this room."
	}
	identity := &inbox.Availability{Name: peer.Name, Role: role.Of(peer.Role).ID, Harness: peer.Harness, Room: peer.Room, CWD: peer.CWD, Existing: existing}
	body := fmt.Sprintf("%s\nname: %s\nrole: %s\nharness: %s\nroom: %s\ncwd: %q", label, identity.Name, identity.Role, identity.Harness, identity.Room, identity.CWD)
	return inbox.Message{ID: id, From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Note, Text: body, CreatedAt: time.Now(), Availability: identity}
}

func announceAvailable(ctx context.Context, dir string, self registry.Session, seen map[string]registry.Session) error {
	current, err := registry.LookupReadOnly(dir, self.Name)
	if err != nil {
		return err
	}
	if current.Epoch() != self.Epoch() || current.Role != role.Main.ID || current.MessagingReadyAt == nil {
		return registry.ErrNotFound
	}
	current.Room = filepath.Base(dir)
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return err
	}
	for _, peer := range sessions {
		if peer.Name == self.Name || peer.MessagingReadyAt == nil {
			continue
		}
		peer.Room = current.Room
		message := availabilityMessage(current, peer)
		if _, known := seen[message.ID]; known {
			continue
		}
		err := putAvailability(ctx, dir, current, peer, message)
		if err == nil {
			seen[message.ID] = peer
		}
	}
	// A temporarily unreadable registry record must not erase dedup evidence.
	// Keep one receipt per registered epoch, including after mailbox retention.
	for id, peer := range seen {
		current, err := registry.Load(dir, peer.Name)
		if errors.Is(err, registry.ErrNotFound) || err == nil && current.Epoch() != peer.Epoch() {
			delete(seen, id)
		}
	}
	return nil
}

// Revalidation inside the receiver mailbox must never take a sender name lock.
func putAvailability(ctx context.Context, dir string, current, peer registry.Session, message inbox.Message) error {
	return putMainNotice(ctx, dir, current, message, func() error {
		subject, err := registry.LookupReadOnly(dir, peer.Name)
		if err != nil || subject.Epoch() != peer.Epoch() || subject.MessagingReadyAt == nil {
			return registry.ErrNotFound
		}
		return nil
	})
}

func putMainNotice(ctx context.Context, dir string, current registry.Session, message inbox.Message, validate func() error) error {
	wait, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	return state.WithMailboxLock(wait, dir, current.Name, func() error {
		target, err := registry.LookupReadOnly(dir, current.Name)
		if err != nil || target.Epoch() != current.Epoch() || target.Role != role.Main.ID {
			return registry.ErrNotFound
		}
		if err := validate(); err != nil {
			return err
		}
		return inbox.PutOnce(dir, message)
	})
}
