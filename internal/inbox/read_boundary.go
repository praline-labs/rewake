package inbox

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/praline-labs/rewake/internal/state"
)

// ReadBoundary is a causal mailbox cutoff, not a wall-clock estimate.
type ReadBoundary struct {
	Mailbox string
	Epoch   string
	Through uint64
}

// ReadClock exposes a single shared committed-read word. Snapshot performs no
// filesystem operations or locks on the protocol reader. Close follows producer shutdown.
type ReadClock struct {
	mailbox string
	epoch   string
	file    *os.File
	data    []byte
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
	file, err := os.OpenFile(filepath.Join(path, ".read-clock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("read counter is not a regular file")
	}
	if info.Size() == 0 {
		err = file.Truncate(8)
	} else if info.Size() != 8 {
		err = errors.New("invalid read-boundary counter")
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, 8, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &ReadClock{mailbox: filepath.Join(dir, name), epoch: epoch, file: file, data: data}, nil
}

// OpenReadClock initializes the mapping before native event production starts.
func OpenReadClock(ctx context.Context, dir, name, epoch string) (clock *ReadClock, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err = state.WithMailboxLock(ctx, dir, name, func() error {
		clock, err = openReadClock(dir, name, epoch)
		if err != nil {
			return err
		}
		through := clock.Snapshot().Through
		for _, waiter := range Waiters(dir, name, epoch) {
			for _, seq := range waiter.ReadSequences {
				through = max(through, seq)
			}
		}
		atomic.StoreUint64(clock.word(), through)
		return nil
	})
	return
}
func (c *ReadClock) word() *uint64 { return (*uint64)(unsafe.Pointer(&c.data[0])) }

// Snapshot captures only the committed word; producers never acquire the mailbox lock.
func (c *ReadClock) Snapshot() *ReadBoundary {
	return &ReadBoundary{Mailbox: c.mailbox, Epoch: c.epoch, Through: atomic.LoadUint64(c.word())}
}

// Close releases this mapping after all completion producers have stopped.
func (c *ReadClock) Close() { _ = syscall.Munmap(c.data); _ = c.file.Close() }

// The mailbox lock serializes writers. Persist waiter sequences before publishing
// the shared word; an earlier captured boundary cannot absorb a later read.
// readAt is when the message was read, 0 for now.
func markScopedAwaiting(dir, name, epoch string, message Message, readAt int64) error {
	if message.FromEpoch == "" || !state.ValidName(message.From) {
		return nil
	}
	c, err := openReadClock(dir, name, epoch)
	if err != nil {
		return err
	}
	defer c.Close()
	next := c.Snapshot().Through
	for _, waiter := range Waiters(dir, name, epoch) {
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
	if err := markAwaitingSequence(dir, name, epoch, message.From, message.FromEpoch, message.ID, next+1, readAt); err != nil {
		return err
	}
	atomic.StoreUint64(c.word(), next+1)
	return c.file.Sync()
}

// ScopedWaiters resolves only still-owed messages committed before the captured
// boundary. Later reads and recipient epochs cannot enter a delayed result.
func ScopedWaiters(dir, name, epoch string, boundary *ReadBoundary) ([]Waiter, error) {
	waiters := Waiters(dir, name, epoch)
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
