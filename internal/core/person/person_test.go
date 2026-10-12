package person

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

const user = 1000

// process is one entry of a /proc fixture. A nil env is an environment the
// kernel keeps from the reader; noStatus is a status that does not read.
type process struct {
	pid, parent, uid int
	start            uint64
	pidNamespace     string
	env              []string
	noStatus         bool
	// noNamespaces leaves the namespace links out, as the kernel keeps
	// another user's from this one.
	noNamespaces bool
}

// procTree writes a /proc-shaped fixture every read of the chain can take.
func procTree(t *testing.T, processes ...process) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range processes {
		writeProcess(t, root, p)
	}
	return root
}

func writeProcess(t *testing.T, root string, p process) {
	t.Helper()
	dir := filepath.Join(root, fmt.Sprint(p.pid))
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	mkdir(t, filepath.Join(dir, "ns"))
	stat := fmt.Sprintf("%d (p %d) S %d%s %d\n", p.pid, p.pid, p.parent, strings.Repeat(" 0", 17), p.start)
	write(t, filepath.Join(dir, "stat"), stat)
	namespace := p.pidNamespace
	if namespace == "" {
		namespace = "pid:[4026531836]"
	}
	for kind, target := range map[string]string{"mnt": "mnt:[4026531841]", "user": "user:[4026531837]", "pid": namespace} {
		if p.noNamespaces {
			break
		}
		if err := os.Symlink(target, filepath.Join(dir, "ns", kind)); err != nil {
			t.Fatal(err)
		}
	}
	// A directory where a file belongs reads as an error for every user,
	// root included, as a file closed to this user does for the kernel's.
	if p.noStatus {
		mkdir(t, filepath.Join(dir, "status"))
	} else {
		write(t, filepath.Join(dir, "status"), fmt.Sprintf("Name:\tp\nUid:\t%d\t%d\t%d\t%d\n", p.uid, p.uid, p.uid, p.uid))
	}
	if p.env == nil {
		mkdir(t, filepath.Join(dir, "environ"))
	} else {
		write(t, filepath.Join(dir, "environ"), strings.Join(p.env, "\x00")+"\x00")
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// init is PID 1 as an ordinary machine shows it to a user: root's, its
// environment closed.
var initProcess = process{pid: 1, uid: 0, start: 1, noNamespaces: true}

// scanOf classifies process 100 against a root with one room, whose records
// the inventory gives.
func scanOf(t *testing.T, tree string, inventory func(string) ([]registry.Inventoried, error)) Scan {
	t.Helper()
	root := t.TempDir()
	mkdir(t, filepath.Join(root, "rooms", "default"))
	if inventory == nil {
		inventory = func(string) ([]registry.Inventoried, error) { return nil, nil }
	}
	return Scan{Proc: proc.Reader{Root: tree}, Root: root, Self: 100, UID: user, inventory: inventory}
}

func records(sessions ...registry.Inventoried) func(string) ([]registry.Inventoried, error) {
	return func(string) ([]registry.Inventoried, error) { return sessions, nil }
}

func wrapperAt(pid int, start uint64, run proc.IdentityEvidence) registry.Inventoried {
	return registry.Inventoried{Session: registry.Session{Name: "lead", ServicePID: pid, ServiceStart: start}, Run: run}
}

// verdict names what Classify answered, as far as a test compares.
func verdict(err error) string {
	var unreadable *UnreadableError
	var below *BelowError
	switch {
	case err == nil:
		return "person"
	case errors.As(err, &below):
		return fmt.Sprintf("below %d", below.PID)
	case errors.As(err, &unreadable):
		return "unreadable"
	}
	return "other: " + err.Error()
}

// The person is found only when every read the answer stands on succeeds;
// each branch of the chain and the registry is held here, with the user's
// rule for processes of another user (main's decision of October 12, 2026):
// their user is read, their environment is not required.
func TestPersonDecisionFailsClosed(t *testing.T) {
	caller := process{pid: 100, parent: 50, uid: user, start: 300, env: []string{"HOME=/home/u"}}
	shell := process{pid: 50, parent: 1, uid: user, start: 200, env: []string{"HOME=/home/u"}}
	cases := map[string]struct {
		processes []process
		inventory func(string) ([]registry.Inventoried, error)
		want      string
	}{
		"outside every session": {
			processes: []process{caller, shell, initProcess}, want: "person",
		},
		"an ancestor of this user whose environment does not read": {
			processes: []process{caller, func() process { p := shell; p.env = nil; return p }(), initProcess},
			want:      "unreadable",
		},
		"an ancestor of another user whose user does not read": {
			processes: []process{caller, shell, func() process { p := initProcess; p.noStatus = true; return p }()},
			want:      "unreadable",
		},
		"an ancestor of this user whose namespaces do not read": {
			processes: []process{caller, func() process { p := shell; p.noNamespaces = true; return p }(), initProcess},
			want:      "unreadable",
		},
		"the caller's own status does not read": {
			processes: []process{func() process { p := caller; p.noStatus = true; return p }(), shell, initProcess},
			want:      "unreadable",
		},
		"a session variable unset in the caller's shell": {
			processes: []process{caller, func() process { p := shell; p.env = []string{"REWAKE_SESSION=lead"}; return p }(), initProcess},
			want:      "below 50",
		},
		"a root ancestor between this user's harness and the caller": {
			processes: []process{
				caller,
				{pid: 50, parent: 40, uid: 0, start: 200},
				{pid: 40, parent: 1, uid: user, start: 150, env: []string{"REWAKE_SESSION=lead"}},
				initProcess,
			},
			want: "below 40",
		},
		"below a live wrapper": {
			processes: []process{caller, shell, initProcess},
			inventory: records(wrapperAt(50, 200, proc.IdentityAlive)), want: "below 50",
		},
		"a wrapper that ended under a pid taken again": {
			processes: []process{caller, shell, initProcess},
			inventory: records(wrapperAt(50, 199, proc.IdentityEnded)), want: "person",
		},
		"a wrapper whose run cannot be judged": {
			processes: []process{caller, shell, initProcess},
			inventory: records(wrapperAt(7, 9, proc.IdentityUnknown)), want: "unreadable",
		},
		"a room whose records do not read": {
			processes: []process{caller, shell, initProcess},
			inventory: func(string) ([]registry.Inventoried, error) { return nil, errors.New("a damaged record") },
			want:      "unreadable",
		},
		"a parent younger than its child, every time": {
			processes: []process{caller, func() process { p := shell; p.start = 400; return p }(), initProcess},
			want:      "unreadable",
		},
		"an ancestor in another pid namespace": {
			processes: []process{caller, func() process { p := shell; p.pidNamespace = "pid:[1]"; return p }(), initProcess},
			want:      "unreadable",
		},
		"a chain that ends short of PID 1": {
			processes: []process{caller, func() process { p := shell; p.parent = 0; return p }()},
			want:      "unreadable",
		},
		"an ancestor gone": {
			processes: []process{caller, initProcess},
			want:      "unreadable",
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := verdict(scanOf(t, procTree(t, c.processes...), c.inventory).Classify())
			if got != c.want {
				t.Fatalf("classified as %s, want %s", got, c.want)
			}
		})
	}
}

// A chain that changes between its two readings is read again; one that
// holds still the second time is judged as it then reads.
func TestPersonScanRestartsWhenTheChainMoves(t *testing.T) {
	caller := process{pid: 100, parent: 50, uid: user, start: 300, env: []string{}}
	shell := process{pid: 50, parent: 1, uid: user, start: 200, env: []string{}}
	tree := procTree(t, caller, shell, initProcess)
	calls := 0
	scan := scanOf(t, tree, func(string) ([]registry.Inventoried, error) {
		calls++
		if calls == 1 {
			// The shell ends and its pid is taken by a process below a
			// session, between the scan's two readings.
			moved := shell
			moved.start, moved.env = 250, []string{"REWAKE_SESSION=lead"}
			writeProcess(t, tree, moved)
		}
		return nil, nil
	})
	if got := verdict(scan.Classify()); got != "below 50" || calls != 2 {
		t.Fatalf("classified as %s after %d scans", got, calls)
	}
}

// A registry that never holds still is refused after the bound, never
// waited out.
func TestPersonScanGivesUpOnAChainThatNeverHoldsStill(t *testing.T) {
	shell := process{pid: 50, parent: 1, uid: user, start: 200, env: []string{}}
	tree := procTree(t, process{pid: 100, parent: 50, uid: user, start: 300, env: []string{}}, shell, initProcess)
	calls := 0
	scan := scanOf(t, tree, func(string) ([]registry.Inventoried, error) {
		calls++
		moved := shell
		moved.start = uint64(200 - calls)
		writeProcess(t, tree, moved)
		return nil, nil
	})
	if got := verdict(scan.Classify()); got != "unreadable" || calls != attempts {
		t.Fatalf("classified as %s after %d scans", got, calls)
	}
}

// Every room of the root is read, and an entry of rooms that is none is no
// room to pass over.
func TestPersonScanReadsEveryRoom(t *testing.T) {
	tree := procTree(t,
		process{pid: 100, parent: 50, uid: user, start: 300, env: []string{}},
		process{pid: 50, parent: 1, uid: user, start: 200, env: []string{}},
		initProcess)
	var seen []string
	scan := scanOf(t, tree, func(room string) ([]registry.Inventoried, error) {
		seen = append(seen, filepath.Base(room))
		if filepath.Base(room) == "other" {
			return []registry.Inventoried{wrapperAt(50, 200, proc.IdentityAlive)}, nil
		}
		return nil, nil
	})
	mkdir(t, filepath.Join(scan.Root, "rooms", "other"))
	if got := verdict(scan.Classify()); got != "below 50" || strings.Join(seen, ",") != "default,other" {
		t.Fatalf("classified as %s over rooms %v", got, seen)
	}
	write(t, filepath.Join(scan.Root, "rooms", "notes.txt"), "")
	if got := verdict(scan.Classify()); got != "unreadable" {
		t.Fatalf("a file among the rooms classified as %s", got)
	}
}

// The real machine: what this test process is depends on where it runs, so
// the answer is logged, and only an answer of no kind fails.
func TestPersonOnThisMachine(t *testing.T) {
	err := Classify(t.TempDir())
	got := verdict(err)
	if strings.HasPrefix(got, "other") {
		t.Fatal(got)
	}
	t.Logf("this test process classifies as %s: %v", got, err)
}
