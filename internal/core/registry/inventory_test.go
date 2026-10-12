package registry

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// observing describes the runs a strict read finds: by pid, running or ended;
// any other pid cannot be told.
func observing(t *testing.T, runs map[int]proc.IdentityEvidence) {
	t.Helper()
	previous := observe
	observe = func(pid int, _ uint64) proc.IdentityEvidence {
		if evidence, ok := runs[pid]; ok {
			return evidence
		}
		return proc.IdentityUnknown
	}
	t.Cleanup(func() { observe = previous })
}

// The strict read takes every record and keeps what it cannot tell apart
// from what it can: a run not known to be running or ended is unknown, never
// one of them.
func TestInventoryKeepsUnknownApartFromEnded(t *testing.T) {
	dir := stateDir(t, map[int]uint64{10: 100, 11: 100, 12: 100})
	observing(t, map[int]proc.IdentityEvidence{10: proc.IdentityAlive, 11: proc.IdentityEnded})
	for _, s := range []Session{session("lead", 10, 100), session("api", 11, 100), session("web", 12, 100)} {
		if err := Publish(dir, s); err != nil {
			t.Fatal(err)
		}
	}
	found, err := Inventory(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]proc.IdentityEvidence{}
	for _, record := range found {
		got[record.Session.Name] = record.Run
	}
	want := map[string]proc.IdentityEvidence{"lead": proc.IdentityAlive, "api": proc.IdentityEnded, "web": proc.IdentityUnknown}
	if len(got) != len(want) {
		t.Fatalf("read %v, want %v", got, want)
	}
	for name, evidence := range want {
		if got[name] != evidence {
			t.Fatalf("%s read as %v, want %v (all: %v)", name, got[name], evidence, got)
		}
	}
	if missing, err := Inventory(filepath.Join(dir, "nowhere")); err != nil || len(missing) != 0 {
		t.Fatalf("a room with no registry yet reads as %v, %v", missing, err)
	}
}

// Every record that does not read, and every entry that is no record, makes
// the read fail naming it: a forgiving view would take it for a session that
// is not there.
func TestInventoryRefusesWhatItCannotRead(t *testing.T) {
	cases := map[string]func(t *testing.T, sessions string){
		"a record that does not parse": func(t *testing.T, sessions string) {
			write(t, filepath.Join(sessions, "web.json"), "{")
		},
		"a record of no process": func(t *testing.T, sessions string) {
			write(t, filepath.Join(sessions, "web.json"), `{"name":"web"}`)
		},
		"a record of another name": func(t *testing.T, sessions string) {
			write(t, filepath.Join(sessions, "web.json"), `{"name":"ops","servicePid":12,"serviceStart":100}`)
		},
		"a record that is a link": func(t *testing.T, sessions string) {
			write(t, filepath.Join(sessions, ".target"), `{"name":"web","servicePid":12,"serviceStart":100}`)
			if err := os.Symlink(filepath.Join(sessions, ".target"), filepath.Join(sessions, "web.json")); err != nil {
				t.Fatal(err)
			}
		},
		"a record that is a directory": func(t *testing.T, sessions string) {
			if err := os.Mkdir(filepath.Join(sessions, "web.json"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"a record that is a FIFO": func(t *testing.T, sessions string) {
			if err := syscall.Mkfifo(filepath.Join(sessions, "web.json"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"a file of no kind": func(t *testing.T, sessions string) {
			write(t, filepath.Join(sessions, "notes.txt"), "x")
		},
		"a record of an unusable name": func(t *testing.T, sessions string) {
			write(t, filepath.Join(sessions, "Web Server.json"), `{"name":"Web Server","servicePid":12,"serviceStart":100}`)
		},
		"a record that cannot be opened": func(t *testing.T, sessions string) {
			path := filepath.Join(sessions, "web.json")
			write(t, path, `{"name":"web","servicePid":12,"serviceStart":100}`)
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			dir := stateDir(t, map[int]uint64{10: 100})
			if err := Publish(dir, session("lead", 10, 100)); err != nil {
				t.Fatal(err)
			}
			if os.Geteuid() == 0 && name == "a record that cannot be opened" {
				t.Skip("root opens a file of mode 0")
			}
			damage(t, state.SessionsPath(dir))
			found, err := Inventory(dir)
			var inventory *InventoryError
			if !errors.As(err, &inventory) {
				t.Fatalf("read %v, %v; want an inventory error", found, err)
			}
		})
	}
	t.Run("a registry that cannot be listed", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root lists a directory of mode 0")
		}
		dir := stateDir(t, map[int]uint64{10: 100})
		if err := Publish(dir, session("lead", 10, 100)); err != nil {
			t.Fatal(err)
		}
		sessions := state.SessionsPath(dir)
		if err := os.Chmod(sessions, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(sessions, 0o700) })
		if _, err := Inventory(dir); err == nil {
			t.Fatal("a registry that cannot be listed reads")
		}
	})
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
