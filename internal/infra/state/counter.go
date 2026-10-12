package state

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// Counter is a guarded resource of the state
// (docs/v2/stage3-state.md#what-is-a-writer): one word of eight bytes in a
// file, mapped shared, so a reader in another goroutine — or another process
// mapping the same file — takes its value without a lock and without the
// filesystem. Its writes are the mapping's: making the file, the stores
// through the word and the flush of the descriptor, each behind the same guard
// as every other write of the room. Loading is no write.
type Counter struct {
	path string
	file *os.File
	data []byte
}

// counterSize is the one word a counter holds.
const counterSize = 8

// OpenCounter maps the counter at path, made empty — zero — when missing. A
// path that is not a regular file of one word is refused rather than
// repaired: it is not a counter this package wrote.
func OpenCounter(path string) (*Counter, error) {
	if err := viewWrite(OpWrite, path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
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
		return nil, fmt.Errorf("the counter %s is not a regular file", path)
	}
	if info.Size() == 0 {
		err = file.Truncate(counterSize)
	} else if info.Size() != counterSize {
		err = fmt.Errorf("the counter %s does not hold one word", path)
	}
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	data, err := syscall.Mmap(int(file.Fd()), 0, counterSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &Counter{path: path, file: file, data: data}, nil
}

func (c *Counter) word() *uint64 { return (*uint64)(unsafe.Pointer(&c.data[0])) }

// Path is the counter's file.
func (c *Counter) Path() string { return c.path }

// Load reads the word: no filesystem call, no lock, so a reader that must
// never block — a protocol reader capturing a boundary — may call it.
func (c *Counter) Load() uint64 { return atomic.LoadUint64(c.word()) }

// Store publishes v through the word. It is a write of the room, guarded as
// one; a refused store leaves the word as it was.
func (c *Counter) Store(v uint64) error {
	if err := viewWrite(OpWrite, c.path); err != nil {
		return err
	}
	atomic.StoreUint64(c.word(), v)
	return nil
}

// Sync flushes the stores made through the mapping to the file.
func (c *Counter) Sync() error {
	if err := viewWrite(OpWrite, c.path); err != nil {
		return err
	}
	return c.file.Sync()
}

// Close unmaps the word and closes its file. The caller closes it once every
// reader that may load it has stopped.
func (c *Counter) Close() error {
	return errors.Join(syscall.Munmap(c.data), c.file.Close())
}
