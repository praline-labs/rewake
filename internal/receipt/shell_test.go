package receipt

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The shell observations (docs/mail-bridge-channel.md#the-shell-observation):
// every kind of entry the directory may hold, folded oldest first, the bound
// keeping the newest, and every error sorted into its class or none.

func TestTakeShellOverEveryEntry(t *testing.T) {
	note := func(boot int64, ok bool) string {
		raw, _ := json.Marshal(ShellNote{OK: ok, Boot: boot, Wall: time.Unix(boot, 0)})
		return string(raw)
	}
	entries := []struct {
		name, body string
		dir        bool
		taken      bool // read as an observation
		left       bool // still there after the take
	}{
		{name: "30.1", body: note(30, true), taken: true},
		{name: "10.2", body: note(10, false), taken: true},
		{name: "20.3", body: note(20, true), taken: true},
		{name: ".tmp-123", body: note(5, true), left: true},
		{name: "0.4", body: note(1, true), left: true},
		{name: "-5.4", body: note(1, true), left: true},
		{name: "x.4", body: note(1, true), left: true},
		{name: "40", body: note(40, true), left: true},
		{name: "50.5", dir: true, left: true},
		{name: "60.6", body: "not json"},
		{name: "70.7", body: note(0, true)},
		{name: "80.8", body: `{"ok":true,"boot":80,"pad":"` + strings.Repeat("x", 1100) + `"}`},
		{name: "99999999999999999999.9", body: note(1, true), left: true},
	}
	for mask := 0; mask < 1<<len(entries); mask += 7 {
		dir := journalDir(t)
		path, err := shellDir(dir, "api", "e1")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		var want []int64
		for index, entry := range entries {
			if mask&(1<<index) == 0 {
				continue
			}
			file := filepath.Join(path, entry.name)
			if entry.dir {
				err = os.Mkdir(file, 0o700)
			} else {
				err = os.WriteFile(file, []byte(entry.body), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if entry.taken {
				want = append(want, map[string]int64{"10.2": 10, "20.3": 20, "30.1": 30}[entry.name])
			}
		}
		for i := 1; i < len(want); i++ {
			for j := i; j > 0 && want[j] < want[j-1]; j-- {
				want[j], want[j-1] = want[j-1], want[j]
			}
		}
		var got []int64
		for _, note := range TakeShell(dir, "api", "e1") {
			got = append(got, note.Boot)
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("mask %b: took %v, want %v", mask, got, want)
		}
		for index, entry := range entries {
			_, err := os.Lstat(filepath.Join(path, entry.name))
			if there := err == nil; mask&(1<<index) != 0 && there != entry.left {
				t.Fatalf("mask %b: %s there %v after the take", mask, entry.name, there)
			}
		}
	}
}

func TestTheBoundKeepsTheNewest(t *testing.T) {
	for _, count := range []int{0, 1, maxShellNotes - 1, maxShellNotes, maxShellNotes + 1, 3 * maxShellNotes} {
		dir := journalDir(t)
		for boot := 1; boot <= count; boot++ {
			WriteShell(dir, "api", "e1", ShellNote{OK: boot%2 == 0, Boot: int64(boot), Wall: time.Now()})
		}
		notes := TakeShell(dir, "api", "e1")
		if len(notes) != min(count, maxShellNotes) || count > 0 && notes[len(notes)-1].Boot != int64(count) {
			t.Fatalf("%d written: took %d, newest %v", count, len(notes), notes)
		}
		path, _ := shellDir(dir, "api", "e1")
		if left, _ := os.ReadDir(path); len(left) != 0 {
			t.Fatalf("%d written: %d left behind", count, len(left))
		}
	}
}

func TestEveryErrorHasItsClass(t *testing.T) {
	wrap := func(errno syscall.Errno) error { return &fs.PathError{Op: "open", Path: "/state/x", Err: errno} }
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{os.ErrNotExist, ""},
		{os.ErrDeadlineExceeded, ""},
		{wrap(syscall.ENOENT), ""},
		{wrap(syscall.EEXIST), ""},
		{wrap(syscall.ENOSPC), ""},
		{wrap(syscall.EROFS), ShellReadOnly},
		{wrap(syscall.EACCES), ShellNotAllowed},
		{wrap(syscall.EPERM), ShellNotAllowed},
		{wrap(syscall.EIO), ShellIOError},
		{fmt.Errorf("lock: %w", wrap(syscall.EROFS)), ShellReadOnly},
	} {
		if got := ShellClass(tc.err); got != tc.want {
			t.Fatalf("%v: class %q, want %q", tc.err, got, tc.want)
		}
	}
}

// A state the CLI cannot write loses the observation and leaves nothing.
func TestAnUnwritableStateLosesTheObservation(t *testing.T) {
	dir := journalDir(t)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	WriteShell(dir, "api", "e1", ShellNote{OK: true, Boot: 1})
	if notes := TakeShell(dir, "api", "e1"); len(notes) != 0 {
		t.Fatalf("took %v", notes)
	}
}
