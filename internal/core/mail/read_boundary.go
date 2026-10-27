package mail

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// ReadBoundary is a causal mailbox cutoff, not a wall-clock estimate.
type ReadBoundary struct {
	Mailbox string
	Epoch   string
	Through uint64
}

// ReadClock exposes a single shared committed-read word, a counter of the
// state (state.Counter). Snapshot performs no filesystem operations or locks
// on the protocol reader. Close follows producer shutdown.
type ReadClock struct {
	mailbox string
	epoch   string
	counter *state.Counter
}

func openReadClock(dir, name, epoch string) (*ReadClock, error) {
	path, ok := awaitingPath(dir, name, epoch)
	if !ok {
		return nil, errors.New("invalid read-boundary epoch")
	}
	if err := state.EnsureSubdir(state.AwaitingPath(dir, name)); err != nil {
		return nil, err
	}
	if err := state.EnsureSubdir(path); err != nil {
		return nil, err
	}
	counter, err := state.OpenCounter(filepath.Join(path, ".read-clock"))
	if err != nil {
		return nil, err
	}
	return &ReadClock{mailbox: filepath.Join(dir, name), epoch: epoch, counter: counter}, nil
}

// OpenReadClock initializes the mapping before native event production starts.
func openClock(ctx context.Context, dir, name, epoch string) (clock *ReadClock, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = state.WithMailboxLock(ctx, dir, name, func() error {
		clock, err = openReadClock(dir, name, epoch)
		if err != nil {
			return err
		}
		through := clock.Snapshot().Through
		// A record that cannot be read may hold the highest sequence: the
		// clock started below it would hand a later read a sequence already
		// taken.
		waiters, err := readWaiters(dir, name, epoch)
		if err != nil {
			clock.Close()
			clock = nil
			return err
		}
		for _, waiter := range waiters {
			for _, seq := range waiter.ReadSequences {
				through = max(through, seq)
			}
		}
		if err := clock.counter.Store(through); err != nil {
			clock.Close()
			clock = nil
			return err
		}
		return nil
	})
	return
}

// Snapshot captures only the committed word; producers never acquire the mailbox lock.
func (c *ReadClock) Snapshot() *ReadBoundary {
	return &ReadBoundary{Mailbox: c.mailbox, Epoch: c.epoch, Through: c.counter.Load()}
}

// Close releases this mapping after all completion producers have stopped.
func (c *ReadClock) Close() { _ = c.counter.Close() }

// The mailbox lock serializes writers. Persist waiter sequences before publishing
// the shared word; an earlier captured boundary cannot absorb a later read.
func markScopedAwaiting(dir, name, epoch string, message Message) error {
	if message.FromEpoch == "" || !state.ValidName(message.From) {
		return nil
	}
	return onReadClock(dir, name, epoch, func(position uint64) error {
		return markAwaitingSequence(dir, name, epoch, message.From, message.FromEpoch, message.ID, position)
	})
}

// onReadClock takes the next position on a run's read clock for one read or
// one hold (docs/delivery-turn-end.md#the-read-clock), under the mailbox lock
// the caller holds, in one order: the position is reserved in the durable
// high-water record, write records it, and only then is the word that
// completions capture their boundary from committed. So a boundary never
// covers a position whose record is not yet written, and a position is never
// issued twice: a failed write leaves a gap, and the high-water keeps it.
func onReadClock(dir, name, epoch string, write func(uint64) error) error {
	c, err := openReadClock(dir, name, epoch)
	if err != nil {
		return err
	}
	defer c.Close()
	next := c.Snapshot().Through
	waiters, err := readWaiters(dir, name, epoch)
	if err != nil {
		return err
	}
	for _, waiter := range waiters {
		for _, seq := range waiter.ReadSequences {
			next = max(next, seq)
		}
	}
	path, _ := awaitingPath(dir, name, epoch)
	highPath := filepath.Join(path, ".read-high")
	if raw, err := readHigh(highPath); err == nil {
		high, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil {
			return errors.New("invalid read-boundary high watermark")
		}
		next = max(next, high)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if next == ^uint64(0) {
		return errors.New("read-boundary counter exhausted")
	}
	if err := state.WriteAtomic(highPath, []byte(strconv.FormatUint(next+1, 10))); err != nil {
		return err
	}
	if err := write(next + 1); err != nil {
		return err
	}
	if err := c.counter.Store(next + 1); err != nil {
		return err
	}
	return c.counter.Sync()
}

// raiseReadClock commits a position a hold reserved and recorded, when a crash
// before its commit left the clock's word below it. The caller holds the
// mailbox lock. No position is allocated, and a word at or above seq stays as
// it is, so no boundary captured for another end is widened.
func raiseReadClock(dir, name, epoch string, seq uint64) error {
	c, err := openReadClock(dir, name, epoch)
	if err != nil {
		return err
	}
	defer c.Close()
	if c.Snapshot().Through >= seq {
		return nil
	}
	if err := c.counter.Store(seq); err != nil {
		return err
	}
	return c.counter.Sync()
}

// ScopedWaiters resolves only still-owed messages committed before the captured
// boundary. Later reads and recipient epochs cannot enter a delayed result.
func scopedWaiters(dir, name, epoch string, boundary *ReadBoundary) ([]Waiter, error) {
	waiters, err := readWaiters(dir, name, epoch)
	if err != nil {
		return nil, err
	}
	if boundary == nil {
		return waiters, nil
	}
	if boundary.Epoch != epoch || boundary.Mailbox != filepath.Join(dir, name) {
		return nil, errors.New("completion read boundary belongs to another mailbox or epoch")
	}
	var selected []Waiter
	for _, waiter := range waiters {
		if len(waiter.ReadSequences) != len(waiter.Messages) {
			return nil, errors.New("waiter has no stable read association")
		}
		ids := make([]string, 0, len(waiter.Messages))
		seqs := make([]uint64, 0, len(waiter.Messages))
		times := make([]int64, 0, len(waiter.Messages))
		for i, id := range waiter.Messages {
			if waiter.ReadSequences[i] == 0 {
				return nil, errors.New("waiter has no stable read association")
			}
			if waiter.ReadSequences[i] > 0 && waiter.ReadSequences[i] <= boundary.Through {
				ids = append(ids, id)
				seqs = append(seqs, waiter.ReadSequences[i])
				times = append(times, waiter.readAt(i))
			}
		}
		if len(ids) > 0 {
			waiter.Messages = ids
			waiter.ReadSequences = seqs
			waiter.ReadAt = times
			selected = append(selected, waiter)
		}
	}
	return selected, nil
}

func readHigh(path string) ([]byte, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 32 {
		return nil, errors.New("invalid read-boundary high watermark")
	}
	return io.ReadAll(io.LimitReader(file, 32))
}
