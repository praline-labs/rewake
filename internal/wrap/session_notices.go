package wrap

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

type knownWorker struct {
	session     registry.Session
	last        sessionstate.Snapshot
	compactions uint64
	departure   *inbox.Message
}

type sessionNotices struct {
	available map[string]registry.Session
	known     map[string]*knownWorker
}

func newSessionNotices() *sessionNotices {
	return &sessionNotices{available: map[string]registry.Session{}, known: map[string]*knownWorker{}}
}

// This extends the existing main observer; neither the worker nor native readers
// perform notification I/O. Last snapshots and receipts are scoped to both epochs.
func (n *sessionNotices) scan(ctx context.Context, dir string, self registry.Session) error {
	current, err := registry.LookupReadOnly(dir, self.Name)
	if err != nil {
		return err
	}
	if current.Epoch() != self.Epoch() || current.Role != role.Main.ID || current.MessagingReadyAt == nil {
		return registry.ErrNotFound
	}
	if err := announceAvailable(ctx, dir, self, n.available); err != nil {
		return err
	}
	for id, peer := range n.available {
		live, err := registry.LookupReadOnly(dir, peer.Name)
		if err != nil || live.Epoch() != peer.Epoch() {
			continue
		}
		if n.known[id] == nil {
			n.known[id] = &knownWorker{session: peer, last: sessionstate.Unknown(peer.Epoch())}
		}
	}
	for id, worker := range n.known {
		latest := sessionstate.Load(dir, worker.session.Name, worker.session.Epoch())
		if latest.PublishedAt != nil {
			worker.last = latest
		}
		if err := n.announceCompactions(ctx, dir, current, worker); err != nil {
			continue
		}
		reason, known := observeDeparture(dir, worker.session)
		if !known {
			continue
		}
		if reason != "" {
			if worker.departure == nil {
				message := departureMessage(current, worker, reason)
				worker.departure = &message
			}
			err := putDepartureNotice(ctx, dir, current, worker.session, *worker.departure)
			if err == nil {
				delete(n.known, id)
				delete(n.available, id)
			}
			continue
		}
		worker.departure = nil
	}
	return nil
}

func compactionMessage(self, peer registry.Session, event sessionstate.CompactionEvent) inbox.Message {
	identity := fmt.Sprintf("compaction\x00%s\x00%s\x00%s\x00%s\x00%d", self.Name, self.Epoch(), peer.Name, peer.Epoch(), event.Sequence)
	sum := sha256.Sum256([]byte(identity))
	return inbox.Message{ID: fmt.Sprintf("%019d-%x", event.ObservedAt.UnixNano(), sum[:12]), From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Note, CreatedAt: time.Now(), Text: fmt.Sprintf("Primary compaction completed (observed count %d).", event.Sequence), Compaction: &inbox.CompactionNotice{Count: event.Sequence, ObservedAt: event.ObservedAt}}
}

func (n *sessionNotices) announceCompactions(ctx context.Context, dir string, self registry.Session, worker *knownWorker) error {
	for _, event := range worker.last.CompactionEvents {
		if event.Sequence <= worker.compactions {
			continue
		}
		if event.ObservedAt.IsZero() || !event.ObservedAt.After(self.StartedAt) {
			worker.compactions = event.Sequence
			continue
		}
		if err := putMainNotice(ctx, dir, self, compactionMessage(self, worker.session, event), func() error { return nil }); err != nil {
			return err
		}
		worker.compactions = event.Sequence
	}
	return nil
}
