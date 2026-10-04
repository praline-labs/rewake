package wrap

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

// The run's channel keeper (docs/mail-bridge-channel.md): it folds what the
// endpoint, the backend and the CLI observe into the channel record, shows
// the record in the session state, and sends the notices the record calls
// for. The record lives in the wrapper's memory; the session state is its
// copy for readers.

// heartbeat is how often the keeper passes timers, folds the shell's
// observations and attempts the notices still to send.
const heartbeat = time.Second

type channelKeeper struct {
	dir, name, epoch string
	harness          channel.Harness

	mu      sync.Mutex
	begun   bool
	record  channel.Record
	notices channel.Notices
	// held are the closes not yet folded. A close waits a full heartbeat
	// after it happened: by then the wrapper knows whether the harness
	// lives, and a server that ended with its harness is no failure.
	// Nothing else waits: the record folds every event by its event time
	// whenever it comes, so a close folded late lands where it happened.
	held  []channel.Event
	alive func() bool
	stop  func()
}

func newChannelKeeper(dir, name, epoch, harnessID string) *channelKeeper {
	return &channelKeeper{dir: dir, name: name, epoch: epoch, harness: channel.Harness(harnessID)}
}

// begin opens the record: the plan decided whether the run has the tool.
func (k *channelKeeper) begin(injected bool, reason string) {
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.record = channel.New(k.harness, injected, reason, endpoint.Stamp())
	k.begun = true
}

// tell takes one event from any source. It never waits on anything but the
// keeper's own lock, which no file operation holds.
func (k *channelKeeper) tell(e channel.Event) {
	if k == nil {
		return
	}
	if e.At.Boot == 0 {
		e.At = endpoint.Stamp()
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.begun {
		return
	}
	switch e.Kind {
	case channel.Exited:
		k.foldExit(e)
	case channel.Closed:
		k.held = append(k.held, e)
	default:
		// A denial among them: the publication guard sees the block as soon
		// as it is told.
		k.record.Fold(e)
	}
}

// foldExit freezes the record. The held closes fold first, each as one the
// harness did not outlive: within a heartbeat of the end, it is the end's,
// not a failure.
func (k *channelKeeper) foldExit(e channel.Event) {
	for _, before := range k.held {
		before.Alive = false
		k.record.Fold(before)
	}
	k.held = nil
	k.record.Fold(e)
}

// foldRipe folds the closes that have waited a full heartbeat, with whether
// the harness lives now; the rest stay held.
func (k *channelKeeper) foldRipe(now channel.Stamp, alive bool) {
	k.held = slices.DeleteFunc(k.held, func(e channel.Event) bool {
		if now.Boot-e.At.Boot < int64(heartbeat) {
			return false
		}
		e.Alive = alive
		k.record.Fold(e)
		return true
	})
}

// started runs the heartbeat until exited. The start the hello timer waits
// from is the harness's own first SessionStart, told by its telemetry.
func (k *channelKeeper) started(ctx context.Context, alive func() bool) {
	if k == nil {
		return
	}
	k.mu.Lock()
	k.alive = alive
	k.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	k.mu.Lock()
	k.stop = func() { cancel(); <-done }
	k.mu.Unlock()
	go func() {
		defer close(done)
		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			k.beat(ctx)
		}
	}()
}

// exited freezes the record: nothing changes and nothing is told after the
// harness is gone, or after the wrapper accepted a signal to end the run.
// Either may come first, and both may come.
func (k *channelKeeper) exited() {
	if k == nil {
		return
	}
	k.tell(channel.Event{Kind: channel.Exited})
	k.mu.Lock()
	stop := k.stop
	k.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// snapshot is the record as readers see it, nil before it is begun.
func (k *channelKeeper) snapshot() *channel.Record {
	if k == nil {
		return nil
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.begun {
		return nil
	}
	record := k.record.View()
	return &record
}

// source adds the record to a session-state source.
func (k *channelKeeper) source(epoch string, source func() sessionstate.Snapshot) func() sessionstate.Snapshot {
	return func() sessionstate.Snapshot {
		snapshot := sessionstate.Unknown(epoch)
		if source != nil {
			snapshot = source()
		}
		snapshot.Channel = k.snapshot()
		return snapshot
	}
}

// beat is one heartbeat: the shell's observations and the timer fold, the
// ripe closes, then the notices.
func (k *channelKeeper) beat(ctx context.Context) {
	notes := receipt.TakeShell(k.dir, k.name, k.epoch)
	main, hasMain := roomMain(k.dir)
	now := endpoint.Stamp()
	k.mu.Lock()
	if k.record.Frozen {
		k.mu.Unlock()
		return
	}
	alive := k.alive == nil || k.alive()
	for _, note := range notes {
		k.record.Fold(channel.Event{Kind: channel.ShellObserved, OK: note.OK, Class: note.Class, At: channel.Stamp{Boot: note.Boot, Wall: note.Wall}})
	}
	// Passing a timer that is not running, or not yet due, changes nothing.
	k.record.Fold(channel.Event{Kind: channel.TimerPassed, At: now})
	k.foldRipe(now, alive)
	record := k.record
	k.notices.Plan(&record, channel.Recipient{Role: channel.ToWorker, Name: k.name, Epoch: k.epoch}, now)
	if hasMain && main.Name != k.name {
		k.notices.Plan(&record, channel.Recipient{Role: channel.ToMain, Name: main.Name, Epoch: main.Epoch()}, now)
	}
	unsent := k.notices.Unsent()
	k.mu.Unlock()
	for _, publication := range unsent {
		k.publish(ctx, publication)
	}
}

// publish attempts one fixed notice, outside the keeper's lock: the
// mailbox lock it takes may wait.
func (k *channelKeeper) publish(ctx context.Context, publication channel.Publication) {
	state, at := k.attempt(ctx, publication)
	if state == "" {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.notices.Settle(publication.Seq, state, at)
}

// attempt answers what came of one attempt: landed, dropped, or "" to try
// again at the next heartbeat.
func (k *channelKeeper) attempt(ctx context.Context, publication channel.Publication) (string, int64) {
	if publication.To.Role == channel.ToMain {
		if main, ok := roomMain(k.dir); !ok || main.Name != publication.To.Name || main.Epoch() != publication.To.Epoch {
			// The main it was fixed for is gone: a new one gets the current
			// category under its own number.
			return channel.Dropped, 0
		}
	}
	message := inbox.Message{
		ID: publication.ID(k.name, k.epoch), From: k.name, FromEpoch: k.epoch,
		To: publication.To.Name, ToEpoch: publication.To.Epoch, Kind: inbox.Note,
		CreatedAt: publication.Fixed.Wall, Text: publication.Body,
	}
	wait, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	_, err := inbox.PublishOnce(wait, k.dir, message, func() error {
		k.mu.Lock()
		defer k.mu.Unlock()
		return k.notices.Writable(&k.record, publication)
	})
	switch {
	case errors.Is(err, channel.ErrNotWritable):
		return channel.Dropped, 0
	case err != nil:
		return "", 0
	}
	return channel.Landed, endpoint.Stamp().Boot
}

// roomMain is the room's live main of this build, if there is one.
func roomMain(dir string) (registry.Session, bool) {
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return registry.Session{}, false
	}
	for _, session := range sessions {
		if role.Of(session.Role).ID == role.Main.ID && !session.EarlierBuild() && session.MessagingReadyAt != nil {
			return session, true
		}
	}
	return registry.Session{}, false
}
