package state

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// failFlush makes every flush of a path that match says fail with EIO for
// the rest of the test, and counts them.
func failFlush(t *testing.T, match func(path string) bool) *int {
	t.Helper()
	failed := 0
	FlushFault = func(path string) error {
		if match(path) {
			failed++
			return syscall.EIO
		}
		return nil
	}
	t.Cleanup(func() { FlushFault = nil })
	return &failed
}

// A publication is durable only when its last flush succeeded. A temporary
// file whose flush failed is never linked and leaves nothing behind; a link
// whose directory flush failed is visible and answers the error, and the name
// stays taken, so a repeat answers ErrNameTaken rather than success — the
// caller that needs it durable has to flush it itself.
func TestPublishExclusiveAnswersEveryFlushItCouldNotMake(t *testing.T) {
	t.Run("the temporary file", func(t *testing.T) {
		dir := leasedDir(t)
		path := filepath.Join(dir, "record")
		failed := failFlush(t, func(at string) bool { return strings.HasPrefix(filepath.Base(at), ".tmp-") })
		if err := PublishExclusive(path, []byte("whole")); !errors.Is(err, syscall.EIO) || *failed != 1 {
			t.Fatalf("the publication answered %v after %d failed flushes", err, *failed)
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("a file that was not flushed was linked: %v", err)
		}
		temps, err := filepath.Glob(filepath.Join(dir, ".tmp-*"))
		if err != nil || len(temps) != 0 {
			t.Fatalf("a publication whose file was not flushed left %v %v", temps, err)
		}
	})
	t.Run("the directory", func(t *testing.T) {
		dir := leasedDir(t)
		path := filepath.Join(dir, "record")
		failed := failFlush(t, func(at string) bool { return at == dir })
		if err := PublishExclusive(path, []byte("whole")); !errors.Is(err, syscall.EIO) || *failed != 1 {
			t.Fatalf("the publication answered %v after %d failed flushes", err, *failed)
		}
		if raw, err := os.ReadFile(path); err != nil || string(raw) != "whole" {
			t.Fatalf("the link is not visible: %q %v", raw, err)
		}
		if err := PublishExclusive(path, []byte("again")); !errors.Is(err, ErrNameTaken) {
			t.Fatalf("a repeat answered %v", err)
		}
		if err := SyncDir(dir); !errors.Is(err, syscall.EIO) {
			t.Fatalf("a later flush of the directory answered %v", err)
		}
	})
}

// SyncFile flushes a regular file and nothing else: a link is not followed,
// a FIFO is not waited on, and a flush that fails is answered.
func TestSyncFileFlushesRegularFilesAlone(t *testing.T) {
	dir := leasedDir(t)
	file := filepath.Join(dir, "record")
	if err := os.WriteFile(file, []byte("whole"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SyncFile(file); err != nil {
		t.Fatalf("a regular file: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(dir, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, fifo, dir} {
		if err := SyncFile(path); !errors.Is(err, ErrNotRegular) {
			t.Errorf("%s: %v", filepath.Base(path), err)
		}
	}
	if err := SyncFile(filepath.Join(dir, "gone")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing file: %v", err)
	}
	failed := failFlush(t, func(at string) bool { return at == file })
	if err := SyncFile(file); !errors.Is(err, syscall.EIO) || *failed != 1 {
		t.Errorf("a flush that failed answered %v", err)
	}
}
