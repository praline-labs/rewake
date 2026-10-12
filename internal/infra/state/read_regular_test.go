package state

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// ReadRegular reads a regular file within its bound and refuses every other
// type at once, by the file it opened: a link is not followed, a FIFO is not
// waited on, a directory is no file. A missing file is an absence.
func TestReadRegularReadsRegularFilesAlone(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "record")
	if err := os.WriteFile(file, []byte("whole"), 0o600); err != nil {
		t.Fatal(err)
	}
	if raw, err := ReadRegular(file, 5); err != nil || string(raw) != "whole" {
		t.Fatalf("a regular file within its bound: %q %v", raw, err)
	}
	if _, err := ReadRegular(file, 4); err == nil || errors.Is(err, ErrNotRegular) {
		t.Fatalf("a file past its bound: %v", err)
	}
	if _, err := ReadRegular(filepath.Join(dir, "gone"), 5); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing file: %v", err)
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
		done := make(chan error, 1)
		go func() { _, err := ReadRegular(path, 5); done <- err }()
		select {
		case err := <-done:
			if !errors.Is(err, ErrNotRegular) {
				t.Errorf("%s: %v", filepath.Base(path), err)
			}
		case <-time.After(3 * time.Second):
			if fd, err := syscall.Open(fifo, syscall.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
				_ = syscall.Close(fd)
			}
			t.Fatalf("%s: the read waited", filepath.Base(path))
		}
	}
}
