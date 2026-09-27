package grantauth

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/proc"
)

// listen serves an authority for a wrapper at self, on a path short enough
// to bind wherever the test runs.
func listen(t *testing.T, self int, lifetime time.Duration) (*Authority, string) {
	t.Helper()
	path := "@rewake-test/" + t.Name() + "/" + strconv.Itoa(os.Getpid())
	authority, err := Listen(path, self, lifetime)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go authority.Serve(ctx)
	t.Cleanup(func() { cancel(); authority.Close() })
	return authority, path
}

func ownExpect(t *testing.T) Expect {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	return Expect{PID: os.Getpid(), Start: start}
}

// stranger is a process that is not this one's ancestor.
func stranger(t *testing.T) int {
	t.Helper()
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	return child.Process.Pid
}

var lib = Grant{ID: "m1", To: "worker", ToEpoch: "e1", Dirs: []string{"/src/lib", "/src/wide"}, Broad: []string{"/src/wide"}}

func TestAGrantRegisteredFromBelowIsConfirmed(t *testing.T) {
	_, path := listen(t, os.Getpid(), time.Minute)
	if err := Register(path, lib); err != nil {
		t.Fatal(err)
	}
	// The same registration again, as a retried send makes it, is no error.
	if err := Register(path, lib); err != nil {
		t.Fatalf("a repeated registration: %v", err)
	}
	got, err := Confirm(path, ownExpect(t), lib.ID, lib.To, lib.ToEpoch)
	if err != nil || !got.Same(lib) {
		t.Fatalf("confirmed %+v, %v", got, err)
	}
	other := lib
	other.Dirs = []string{"/elsewhere"}
	if err := Register(path, other); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("another grant under the same message: %v", err)
	}
}

// A process that does not run below the wrapper — a worker's, which runs
// below its own — cannot register a grant in main's name.
func TestARegistrationFromOutsideTheWrapperIsRefused(t *testing.T) {
	_, path := listen(t, stranger(t), time.Minute)
	err := Register(path, lib)
	if !errors.Is(err, ErrNotConfirmed) || !strings.Contains(err.Error(), "only a command this session runs") {
		t.Fatalf("registered from outside: %v", err)
	}
}

func TestAConfirmationSettlesOnlyForItsRun(t *testing.T) {
	_, path := listen(t, os.Getpid(), time.Minute)
	if err := Register(path, lib); err != nil {
		t.Fatal(err)
	}
	for name, asked := range map[string][3]string{
		"another message":   {"m2", lib.To, lib.ToEpoch},
		"another recipient": {lib.ID, "other", lib.ToEpoch},
		"another run":       {lib.ID, lib.To, "e2"},
	} {
		if _, err := Confirm(path, ownExpect(t), asked[0], asked[1], asked[2]); !errors.Is(err, ErrNotConfirmed) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The answer counts only from the process the sender's record names: a
// listener in the socket's place is somebody else.
func TestAConfirmationFromAnotherProcessIsRefused(t *testing.T) {
	_, path := listen(t, os.Getpid(), time.Minute)
	if err := Register(path, lib); err != nil {
		t.Fatal(err)
	}
	own := ownExpect(t)
	for name, expect := range map[string]Expect{
		"another pid":        {PID: stranger(t), Start: own.Start},
		"another start time": {PID: own.PID, Start: own.Start + 1},
		"no pid":             {},
	} {
		if _, err := Confirm(path, expect, lib.ID, lib.To, lib.ToEpoch); !errors.Is(err, ErrNotConfirmed) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAWrapperNotThereIsUnreachable(t *testing.T) {
	path := "@rewake-test/gone/" + strconv.Itoa(os.Getpid())
	if _, err := Confirm(path, ownExpect(t), lib.ID, lib.To, lib.ToEpoch); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("confirmed by nobody: %v", err)
	}
	if err := Register(path, lib); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("registered with nobody: %v", err)
	}
}

// Past the most it holds, a main refuses the next grant and keeps the ones
// it has: a grant pushed out would be one nobody could confirm.
func TestAFullAuthorityRefusesAndKeepsWhatItHolds(t *testing.T) {
	_, path := listen(t, os.Getpid(), time.Minute)
	for index := range maxHeld {
		grant := lib
		grant.ID = "m" + string(rune('a'+index%26)) + time.Duration(index).String()
		if err := Register(path, grant); err != nil {
			t.Fatalf("registration %d: %v", index, err)
		}
	}
	extra := lib
	extra.ID = "extra"
	if err := Register(path, extra); err == nil || !strings.Contains(err.Error(), "the most it keeps") {
		t.Fatalf("past the limit: %v", err)
	}
	first := "ma" + time.Duration(0).String()
	if _, err := Confirm(path, ownExpect(t), first, lib.To, lib.ToEpoch); err != nil {
		t.Fatalf("the first grant was pushed out: %v", err)
	}
}

func TestAGrantOutlivingItsWaitIsForgotten(t *testing.T) {
	_, path := listen(t, os.Getpid(), 20*time.Millisecond)
	if err := Register(path, lib); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := Confirm(path, ownExpect(t), lib.ID, lib.To, lib.ToEpoch); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("confirmed after its lifetime: %v", err)
	}
}

// A wrapper that ended confirms nothing, and its address is free again.
func TestClosingFreesTheAddress(t *testing.T) {
	authority, path := listen(t, os.Getpid(), time.Minute)
	if err := Register(path, lib); err != nil {
		t.Fatal(err)
	}
	authority.Close()
	if _, err := Confirm(path, ownExpect(t), lib.ID, lib.To, lib.ToEpoch); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("confirmed after closing: %v", err)
	}
	again, err := Listen(path, os.Getpid(), time.Minute)
	if err != nil {
		t.Fatalf("the address stayed taken: %v", err)
	}
	again.Close()
}

// An address already bound — by a listener put there before main started —
// is not taken over: main then answers for nothing, and says so.
func TestABoundAddressIsNotTakenOver(t *testing.T) {
	_, path := listen(t, os.Getpid(), time.Minute)
	if _, err := Listen(path, os.Getpid(), time.Minute); err == nil {
		t.Fatal("a second listener bound the same address")
	}
}

// A wrapper in other namespaces than the one asking is a process a sandbox
// started: a worker inside one could name its own process as the sender's
// run and answer for it, so nothing it confirms counts.
func TestAConfirmationFromOtherNamespacesIsRefused(t *testing.T) {
	_, path := listen(t, os.Getpid(), time.Minute)
	if err := Register(path, lib); err != nil {
		t.Fatal(err)
	}
	own := ownExpect(t)
	root := t.TempDir()
	for pid, mount := range map[string]string{"self": "mnt:[1]", strconv.Itoa(own.PID): "mnt:[2]"} {
		dir := filepath.Join(root, pid, "ns")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for kind, link := range map[string]string{"mnt": mount, "user": "user:[1]", "pid": "pid:[1]"} {
			if err := os.Symlink(link, filepath.Join(dir, kind)); err != nil {
				t.Fatal(err)
			}
		}
	}
	saved := namespaces
	namespaces = proc.Reader{Root: root}.Namespaces
	t.Cleanup(func() { namespaces = saved })
	_, err := Confirm(path, own, lib.ID, lib.To, lib.ToEpoch)
	if !errors.Is(err, ErrNotConfirmed) || !strings.Contains(err.Error(), "other namespaces") {
		t.Fatalf("confirmed from other namespaces: %v", err)
	}
}
