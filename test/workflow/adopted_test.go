package workflow

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// errGone marks a process that disappeared between being listed and being
// read. It is the only kind of /proc failure that means "nothing to see".
var errGone = errors.New("process gone")

// A process that calls setsid leaves the group it was started in, and the
// original group id then proves nothing about it. This is not hypothetical
// here: the Codex adapter starts the native app-server in a group of its own
// (internal/harness/codex/server.go), so on the first real scenario a session
// could go on running while the case reported a clean finish.
//
// Identifiers do not survive setsid, so the group cannot be followed. What
// does survive is descent: an orphan is re-parented to the nearest ancestor
// marked as a child subreaper. The suite marks itself, so every descendant
// whose own parent has gone becomes a direct child of this process and can be
// found, ended and buried.
//
// Cases run in parallel, so descent alone no longer says whose a process is:
// each sweep takes only the descendants carrying its owner label, and the
// rest is left to the final sweep (owner_labels_test.go).

// prSetChildSubreaper is PR_SET_CHILD_SUBREAPER.
const prSetChildSubreaper = 36

// becomeSubreaper makes this process adopt its orphaned descendants.
func becomeSubreaper() error {
	if _, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0, 0, 0, 0); errno != 0 {
		return fmt.Errorf("PR_SET_CHILD_SUBREAPER: %w", errno)
	}
	return nil
}

// stray is a descendant of this process that nothing is accounting for.
type stray struct {
	pid  int
	pgid int
}

// strayDescendants walks the process tree down from this process and returns
// everything below it that carries the label and that the caller does not
// already own; an empty label takes everything, as the final sweep does. It
// also returns the dead among this process's direct children, which only this
// process can bury and whose environment no longer says whose they were.
//
// Descent, not adoption alone: a descendant whose intermediate parent is still
// alive has not been re-parented yet, and looking only at direct children
// would race with that hand-over. Walking the tree finds it either way, and
// subreaper status is what keeps the tree from being cut when the parent does
// exit. The owned processes themselves are skipped, their children are not —
// those are what this is looking for.
func strayDescendants(label string, known map[int]bool) ([]stray, []int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, nil, err
	}
	type node struct {
		ppid, pgid int
		zombie     bool
	}
	all := map[int]node{}
	children := map[int][]int{}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		ppid, pgid, state, err := readStat(pid)
		if errors.Is(err, errGone) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("reading the process tree: %w", err)
		}
		all[pid] = node{ppid: ppid, pgid: pgid, zombie: state == "Z"}
		children[ppid] = append(children[ppid], pid)
	}
	self := os.Getpid()
	var walk func(int)
	var found []stray
	var dead []int
	seen := map[int]bool{}
	walk = func(pid int) {
		for _, child := range children[pid] {
			if seen[child] {
				continue
			}
			seen[child] = true
			if all[child].zombie && pid == self {
				dead = append(dead, child)
			}
			if !known[child] && (label == "" || carries(child, label)) {
				found = append(found, stray{pid: child, pgid: all[child].pgid})
			}
			walk(child)
		}
	}
	walk(self)
	return found, dead, nil
}

// parentAndGroup reads the parent and group of a process from /proc. A
// process that has gone between listing and reading is reported as absent;
// any other failure is a failure, because "could not read" must never be
// delivered as "nothing to see".
func parentAndGroup(pid int) (int, int, error) {
	ppid, pgid, _, err := readStat(pid)
	return ppid, pgid, err
}

// readStat is parentAndGroup with the process's state beside them.
func readStat(pid int) (int, int, string, error) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	// Two ways a process can vanish between being listed and being read: the
	// directory is gone (ENOENT), or the kernel refuses the read because the
	// process behind it has exited (ESRCH). Neither is a failure to look.
	if os.IsNotExist(err) || errors.Is(err, syscall.ESRCH) {
		return 0, 0, "", errGone
	}
	if err != nil {
		return 0, 0, "", err
	}
	// The command name is in parentheses and may itself contain spaces, so the
	// fields after it are counted from the last ')' rather than from the start.
	nameEnd := strings.LastIndex(string(raw), ")")
	if nameEnd < 0 {
		return 0, 0, "", fmt.Errorf("unreadable stat for %d", pid)
	}
	fields := strings.Fields(string(raw)[nameEnd+1:])
	// After the name come state, ppid, pgrp.
	if len(fields) < 3 {
		return 0, 0, "", fmt.Errorf("unreadable stat for %d", pid)
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, "", err
	}
	pgid, err := strconv.Atoi(fields[2])
	if err != nil {
		return 0, 0, "", err
	}
	return ppid, pgid, fields[0], nil
}

// groupHasLiveMember reports whether any process in the group is still
// running, as opposed to waiting to be buried.
//
// kill(2) answers first because it is cheap, and its ESRCH is conclusive. Its
// success is not: a zombie answers exactly like a running process, so the
// group is then confirmed against /proc, where a state of Z is visible.
func groupHasLiveMember(pgid int) (bool, error) {
	if pgid <= 0 {
		return false, nil
	}
	if err := syscall.Kill(-pgid, 0); errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, fmt.Errorf("reading the process tree: %w", err)
	}
	for _, entry := range entries {
		pid, convErr := strconv.Atoi(entry.Name())
		if convErr != nil {
			continue
		}
		_, memberGroup, statErr := parentAndGroup(pid)
		if errors.Is(statErr, errGone) {
			continue
		}
		if statErr != nil {
			return false, statErr
		}
		if memberGroup != pgid {
			continue
		}
		if isZombie(pid) && pid != pgid {
			// Dead, and only waiting to be collected — by this process when
			// it adopted the orphan, so collect it now. A shim that exits
			// while a hook it started is finishing leaves exactly that, and
			// counting it as a member kept a case waiting for a session that
			// had long ended. Never the group's leader: that is the process
			// the case started, whose own Wait collects it and its exit code,
			// and taking it here would leave that Wait with nothing.
			buryUnowned(pid)
			continue
		}
		return true, nil
	}
	return false, nil
}

// straySettle is the one pause this sweep takes before calling a run clean.
// /proc answers about an instant, while a parent exiting, its child being
// re-parented here and that child starting work of its own all happen
// independently of each other: a snapshot taken immediately after a parent is
// reaped can miss a descendant that is about to appear in the tree, and one
// taken while a descendant is exiting can miss it on its way out. So the pause
// runs from the start, from the end of the case's own group and from the last
// find, and a clean tree costs this much once.
const straySettle = 50 * time.Millisecond

// sweepOnce takes one look at the tree, signals what it finds under the label
// and buries whatever has died. A zero signal only looks and reaps, which is
// what the waiting loops need.
//
// A stray is described before it is signaled and the first description is
// kept in described, across the passes of one cleanup: by the time anyone
// reads the report the process is gone, and a pid alone then names nothing.
func sweepOnce(label string, known map[int]bool, signal syscall.Signal, described map[int]string) ([]string, error) {
	strays, dead, err := strayDescendants(label, known)
	if err != nil {
		return nil, err
	}
	for _, pid := range dead {
		buryUnowned(pid)
	}
	var found []string
	for _, s := range strays {
		if _, ok := described[s.pid]; !ok {
			described[s.pid] = describeProcess(s.pid)
		}
		// The final sweep's every find is a failure, including one that dies
		// at the first signal: asked only after signaling, a leftover that
		// ends promptly would be ended and never named, and the run would
		// pass with it. A case's sweep keeps asking after, as it always has.
		runningAtSight := label == "" && processRunning(s.pid)
		if signal != 0 {
			signalStray(s, signal)
		}
		buryUnowned(s.pid)
		if runningAtSight || processRunning(s.pid) {
			found = append(found, fmt.Sprintf("pid %d (group %d) was still running: %s", s.pid, s.pgid, described[s.pid]))
		}
	}
	return found, nil
}

// signalStray signals a stray and its group, unless the group is this
// process's own. A descendant started without a group of its own shares the
// test process's group, and with it go test's and whatever shell ran it:
// signaling that group ends the run and its caller, not the stray.
func signalStray(s stray, signal syscall.Signal) {
	if s.pgid > 0 && s.pgid != syscall.Getpgrp() {
		_ = syscall.Kill(-s.pgid, signal)
	}
	_ = syscall.Kill(s.pid, signal)
}

// processRunning reports whether the pid is still doing something.
//
// Two things are not "running" even though kill(2) accepts them: an absent
// process (ESRCH), and a zombie — a record left for somebody to collect. A
// zombie belonging to a descendant this process cannot reap is collected by
// the kernel when the test binary exits, so treating one as live work makes a
// cleanup loop wait out its whole budget for a process that is already dead.
// EPERM, by contrast, means it exists and is somebody else's: an answer.
func processRunning(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return false
	}
	return !isZombie(pid)
}

// isZombie reads the state field of /proc/<pid>/stat. An unreadable process
// is treated as gone rather than as a zombie: the distinction only matters for
// something that exists.
func isZombie(pid int) bool {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	nameEnd := strings.LastIndex(string(raw), ")")
	if nameEnd < 0 {
		return false
	}
	fields := strings.Fields(string(raw)[nameEnd+1:])
	return len(fields) > 0 && fields[0] == "Z"
}
