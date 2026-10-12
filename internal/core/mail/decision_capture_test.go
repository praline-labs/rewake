package mail

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The capture keeps every read as the reader got it, in order, the failures
// with what lstat said; what it keeps no later change touches, it writes
// nothing, and a plan that reads past the bounds keeps nothing.
func TestDecisionObservationsAreTheFinalReads(t *testing.T) {
	dir := stateDir(t)
	box := state.InboxPath(dir, "api")
	if err := state.EnsureSubdir(box); err != nil {
		t.Fatal(err)
	}
	letter := filepath.Join(box, "letter.json")
	if err := state.WriteAtomic(letter, []byte(`{"id":"x"}`)); err != nil {
		t.Fatal(err)
	}
	files := newCapture(osAccess{}, dir)
	got, _ := files.ReadFile(letter)
	got[0] = 'X'
	if _, err := files.ReadDir(box); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Stat(letter); err != nil {
		t.Fatal(err)
	}
	if _, err := files.Lstat(letter); err != nil {
		t.Fatal(err)
	}
	if _, err := files.ReadFile(filepath.Join(box, "gone.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing file read: %v", err)
	}
	if _, err := files.ReadFile(filepath.Join(letter, "below")); err == nil {
		t.Fatal("a path through a file read")
	}
	if _, err := files.Lstat(filepath.Join(box, "gone.json")); err == nil {
		t.Fatal("a missing file lstat")
	}
	if err := state.WriteAtomic(letter, []byte(`{"id":"y"}`)); err != nil {
		t.Fatal(err)
	}

	observations, err := files.Observations()
	if err != nil {
		t.Fatal(err)
	}
	ops := ""
	for _, o := range observations {
		ops += o.Op + " "
	}
	if ops != "read list stat lstat read read lstat " {
		t.Fatalf("the reads kept are %q", ops)
	}
	if first := observations[0]; !bytes.Equal(first.Bytes, []byte(`{"id":"x"}`)) || first.Path != filepath.Join("inbox", "api", "letter.json") {
		t.Fatalf("the first read kept %q at %s, not what the reader got", first.Bytes, first.Path)
	}
	if gone := observations[4]; gone.Err.Kind != errNotExist || gone.Lstat == nil || gone.Lstat.Err.Kind != errNotExist {
		t.Fatalf("a missing file kept %+v", gone)
	}
	if through := observations[5]; through.Err.Kind != errNotDir || through.Lstat == nil || through.Lstat.Err == nil {
		t.Fatalf("a path through a file kept %+v", through)
	}
	if missing := observations[6]; missing.Err == nil || missing.Lstat != nil {
		t.Fatalf("a failed lstat kept %+v", missing)
	}
	if err := checkObservations(observations); err != nil {
		t.Fatalf("the reads kept do not hold together: %v", err)
	}
	observations[0].Bytes[0] = 'Z'
	again, _ := files.Observations()
	if again[0].Bytes[0] != '{' {
		t.Fatal("the observations handed out are the capture's own")
	}

	for name, write := range map[string]func() error{
		"write":   func() error { return files.WriteFile(letter, nil) },
		"publish": func() error { return files.Publish(letter, nil) },
		"make":    func() error { return files.EnsureDir(box) },
		"remove":  func() error { return files.Remove(letter) },
		"rename":  func() error { return files.Rename(letter, letter+"2") },
		"sync":    func() error { return files.SyncDir(box) },
	} {
		if write() == nil {
			t.Errorf("a capture's %s went through", name)
		}
	}

	big := filepath.Join(box, "big.json")
	if err := state.WriteAtomic(big, make([]byte, maxObservedRead+1)); err != nil {
		t.Fatal(err)
	}
	over := newCapture(osAccess{}, dir)
	if _, err := over.ReadFile(big); err != nil {
		t.Fatal(err)
	}
	if _, err := over.Observations(); !errors.Is(err, ErrObservationBound) {
		t.Fatalf("a read past its bound was kept: %v", err)
	}
	if err := os.Remove(big); err != nil {
		t.Fatal(err)
	}
}
