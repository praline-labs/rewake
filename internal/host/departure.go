package host

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// The boolean says whether the observation is conclusive. Unknown must preserve
// the known worker and any pending notice identity for a later retry.
func observeDeparture(dir string, peer registry.Session) (string, bool) {
	current, err := registry.Load(dir, peer.Name)
	if errors.Is(err, registry.ErrNotFound) {
		return "not registered", true
	}
	if err != nil {
		return "", false
	}
	if current.Epoch() != peer.Epoch() {
		return "replaced by a new run", true
	}
	if !current.Judgeable() {
		return "", false
	}
	service := proc.ObserveIdentity(current.ServicePID, current.ServiceStart)
	harness := proc.IdentityAlive
	if current.HarnessPID != 0 {
		harness = proc.ObserveIdentity(current.HarnessPID, current.HarnessStart)
	}
	if service == proc.IdentityEnded || harness == proc.IdentityEnded {
		return "process ended", true
	}
	if service == proc.IdentityUnknown || harness == proc.IdentityUnknown {
		return "", false
	}
	return "", true
}

func departureReason(dir string, peer registry.Session) string {
	reason, _ := observeDeparture(dir, peer)
	return reason
}

func putDepartureNotice(ctx context.Context, dir string, current, peer registry.Session, message mail.Message) error {
	return putMainNotice(ctx, dir, current, message, func() error {
		if departureReason(dir, peer) == "" {
			return registry.ErrNotFound
		}
		return nil
	})
}

func departureMessage(self registry.Session, worker *knownWorker, reason string) mail.Message {
	peer := worker.session
	sum := sha256.Sum256([]byte("departure\x00" + self.Name + "\x00" + self.Epoch() + "\x00" + peer.Name + "\x00" + peer.Epoch()))
	now := time.Now()
	snapshot := worker.last
	snapshot.Epoch = peer.Epoch()
	snapshot.Stale()
	snapshot.CompactionEvents, snapshot.CompactionOutcomes = nil, nil
	identity := mail.Availability{Name: peer.Name, Role: role.Of(peer.Role).ID, Harness: peer.Harness, Room: peer.Room, CWD: peer.CWD}
	body := fmt.Sprintf("Session is no longer available in this room (%s).\nname: %s\nrole: %s\nharness: %s\nroom: %s\nlaunch cwd: %q (a checkout the worker entered later is in rewake worktree ls)", reason, identity.Name, identity.Role, identity.Harness, identity.Room, identity.CWD)
	// The observer retains this message across failed publication attempts, including
	// its timestamp. PutOnce protects a write that succeeded before reporting error.
	return mail.Message{ID: fmt.Sprintf("%019d-%x", now.UnixNano(), sum[:12]), From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: mail.Note, CreatedAt: now, Text: body, Departure: &mail.DepartureNotice{Identity: identity, Reason: reason}, SenderState: &snapshot}
}
