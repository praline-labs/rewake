package state

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A listing of the clearing opens nothing but a directory of its own: a FIFO
// where a directory was checked is refused by the open without waiting for a
// writer, and a link to a directory is refused, never followed.
func TestAPlainDirListingRefusesWithoutWaitingOrFollowing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	for _, name := range []string{"b", "a"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := ReadPlainDir(dir)
	if err != nil || len(entries) != 2 || entries[0].Name() != "a" || entries[1].Name() != "b" {
		t.Fatalf("a directory listed %v: %v", entries, err)
	}
	fifo := filepath.Join(root, "fifo")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ReadPlainDir(fifo)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "is not a directory") {
			t.Fatalf("a FIFO listed: %v", err)
		}
	case <-time.After(3 * time.Second):
		if writer, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = writer.Close()
		}
		<-done
		t.Fatal("the listing waited on a FIFO for a writer")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if entries, err := ReadPlainDir(link); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Fatalf("a link to a directory listed %v: %v", entries, err)
	}
}
