package sessionstate

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

const (
	maxSnapshotBytes = 16 << 10
	interval         = 250 * time.Millisecond
)

func snapshotPath(dir, name, epoch string) string {
	sum := sha256.Sum256([]byte(name + "\x00" + epoch))
	return filepath.Join(dir, "observations", fmt.Sprintf("%x.json", sum[:]))
}

// Save replaces one bounded latest snapshot; its caller is the wrapper worker,
// never a native reader or a mailbox delivery path.
func Save(dir, name, epoch string, snapshot Snapshot) error {
	if !state.ValidName(name) || epoch == "" {
		return fmt.Errorf("invalid observation identity")
	}
	snapshot.Epoch = epoch
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if len(raw) > maxSnapshotBytes {
		return fmt.Errorf("observation too large")
	}
	path := snapshotPath(dir, name, epoch)
	if err := state.EnsureSubdir(filepath.Dir(path)); err != nil {
		return err
	}
	return state.WriteAtomic(path, raw)
}

// Load never falls back from an old epoch to the current holder of its name.
func Load(dir, name, epoch string) Snapshot {
	unknown := Unknown(epoch)
	if !state.ValidName(name) || epoch == "" {
		return unknown
	}
	path := snapshotPath(dir, name, epoch)
	if state.Verify(filepath.Dir(path)) != nil {
		return unknown
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return unknown
	}
	file := os.NewFile(uintptr(fd), path)
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSnapshotBytes {
		return unknown
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxSnapshotBytes+1))
	var snapshot Snapshot
	if err != nil || len(raw) > maxSnapshotBytes || json.Unmarshal(raw, &snapshot) != nil || snapshot.Epoch != epoch {
		return unknown
	}
	if snapshot.PublishedAt == nil || snapshot.PublishedAt.IsZero() || time.Since(*snapshot.PublishedAt) > 2*time.Second || time.Until(*snapshot.PublishedAt) > time.Second {
		snapshot.Stale()
	}
	return snapshot
}

// Start keeps one worker and no update backlog. Storage failures lose telemetry,
// never delivery. Shutdown does not wait indefinitely on a stalled filesystem.
func Start(parent context.Context, dir, name, epoch string, source func() Snapshot) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			snapshot := source()
			now := time.Now()
			snapshot.PublishedAt = &now
			_ = Save(dir, name, epoch, snapshot)
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
