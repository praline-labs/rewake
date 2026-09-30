package wrap

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

// heldDeliveries tells main once when a worker starts holding its deliveries,
// and once more each time its reason, its conversations or its cause change: senders see
// pending and nothing else, and only the person can clear an unintended
// conversation (docs/delivery-conversation.md). A hold that ends says nothing — the mail
// simply goes — and one that comes back later is told again.
type heldDeliveries struct {
	// told is the hold last told, as holdKey renders it; empty while none.
	told string
	// count numbers the holds told, so each has its own message ID.
	count uint64
	// at is when the hold being told was first seen, kept across retries so
	// a notice that failed to be put goes again under the same ID.
	at time.Time
}

func holdKey(hold *sessionstate.DeliveryHold) string {
	if hold == nil {
		return ""
	}
	// The detail too: a hold of one reason may change its cause — the
	// admission record failing with one error, then another — and main is
	// told of each.
	return hold.Reason + "\x00" + hold.Expected + "\x00" + hold.Selected + "\x00" + hold.Detail
}

func (n *sessionNotices) announceHold(ctx context.Context, dir string, self registry.Session, worker *knownWorker) error {
	key := holdKey(worker.last.DeliveryHold)
	if key == worker.held.told {
		return nil
	}
	if key == "" {
		worker.held.told, worker.held.at = "", time.Time{}
		return nil
	}
	if worker.held.at.IsZero() {
		worker.held.at = time.Now()
	}
	message := holdMessage(self, worker.session, *worker.last.DeliveryHold, worker.held.count+1, worker.held.at)
	if err := putMainNotice(ctx, dir, self, message, func() error { return nil }); err != nil {
		return err
	}
	worker.held.told, worker.held.at = key, time.Time{}
	worker.held.count++
	return nil
}

func holdMessage(self, peer registry.Session, hold sessionstate.DeliveryHold, count uint64, at time.Time) inbox.Message {
	identity := fmt.Sprintf("hold\x00%s\x00%s\x00%s\x00%s\x00%d\x00%s", self.Name, self.Epoch(), peer.Name, peer.Epoch(), count, holdKey(&hold))
	sum := sha256.Sum256([]byte(identity))
	return inbox.Message{
		ID: fmt.Sprintf("%019d-%x", at.UnixNano(), sum[:12]), From: peer.Name, FromEpoch: peer.Epoch(),
		To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Note, CreatedAt: time.Now(),
		Text: fmt.Sprintf("Rewake: deliveries to %s wait: %s", peer.Name, hold.Detail),
	}
}
