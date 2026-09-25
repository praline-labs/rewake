package workflow

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
)

// Cases run in parallel, so "a descendant of this process" no longer means
// "this case's". Every process a case or a bounded preparation starts carries
// an owner label in its environment, and a sweep takes as its own exactly the
// descendants that carry its label, leaving a neighbour's alone.
//
// Environments are inherited down the tree, through setsid and re-parenting
// alike: a process that left its group still says whose it is. The label is a
// variable of a name of its own rather than one value of a shared name, so a
// nested scope — a build started inside a self-check child — adds its label
// beside the outer one; os/exec keeps only the last of two values of one name.
//
// What carries no label, or cannot be read, belongs to no scope. No case sweep
// touches it, and the sweep TestMain runs once every case has finished ends it
// and fails the run: nothing a case started may outlive the run, and the
// guarantee moved there rather than going away.

// ownerLabelPrefix begins every owner label's variable name.
const ownerLabelPrefix = "REWAKE_WORKFLOW_OWNER_"

var ownerLabels atomic.Int64

// newOwnerLabel is a fresh label, as the environment entry it is set by. The
// test process's pid keeps a nested suite's labels apart from its parent's.
func newOwnerLabel() string {
	return fmt.Sprintf("%s%d_%d=1", ownerLabelPrefix, os.Getpid(), ownerLabels.Add(1))
}

// withLabel is env with the label added; a nil env is the test process's own,
// which is what an exec.Cmd without one would have run with.
func withLabel(env []string, label string) []string {
	if env == nil {
		env = os.Environ()
	}
	return append(slices.Clip(env), label)
}

// carries reports whether a process runs under the label. A process that
// cannot be read — gone, or a zombie, whose environment reads empty — does not:
// it is nobody's here, and the final sweep answers for it.
func carries(pid int, label string) bool {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
	if err != nil {
		return false
	}
	for entry := range bytes.SplitSeq(raw, []byte{0}) {
		if string(entry) == label {
			return true
		}
	}
	return false
}

// describeProcess names a process for a report: its command line and the
// owner labels it carries. A process the final sweep ends is gone before
// anyone reads why the run failed, so this is the only account of it there
// will be.
func describeProcess(pid int) string {
	dir := filepath.Join("/proc", strconv.Itoa(pid))
	command := "command line unreadable"
	if raw, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil && len(raw) > 0 {
		command = strconv.Quote(firstRunes(strings.TrimSpace(strings.ReplaceAll(string(raw), "\x00", " ")), 160))
	}
	var labels []string
	if raw, err := os.ReadFile(filepath.Join(dir, "environ")); err == nil {
		for entry := range strings.SplitSeq(string(raw), "\x00") {
			if name, _, ok := strings.Cut(entry, "="); ok && strings.HasPrefix(name, ownerLabelPrefix) {
				labels = append(labels, name)
			}
		}
	}
	if len(labels) == 0 {
		return command + ", no owner label"
	}
	return command + ", owner " + strings.Join(labels, " ")
}

// firstRunes cuts text to at most n runes, marking the cut.
func firstRunes(text string, n int) string {
	runes := []rune(text)
	if len(runes) <= n {
		return text
	}
	return string(runes[:n]) + "…"
}

// leaders are the processes this test process started and whose exit status
// a Wait of its own is collecting. A zombie's environment is empty, so a
// sweep cannot tell a neighbour's leader waiting to be collected from an
// adopted orphan by its label; the registry tells it, and no sweep takes a
// leader's status from the Wait that owns it.
//
// It does not bind a case's cleanup of its own group: reapGroup waits on the
// whole group, leader included, so a leader still running when its own case
// ends it — a timeout, a deadline, a process left at the end, all red already
// — can be collected there, and its Wait then answers with an error rather
// than the status.
var leaders = struct {
	mu   sync.Mutex
	pids map[int]bool
}{pids: map[int]bool{}}

// startLeader starts cmd and registers it in one step: a process that exited
// between the two could otherwise be buried as an orphan before it was known.
func startLeader(cmd *exec.Cmd) error {
	leaders.mu.Lock()
	defer leaders.mu.Unlock()
	if err := cmd.Start(); err != nil {
		return err
	}
	leaders.pids[cmd.Process.Pid] = true
	return nil
}

// forgetLeader is called once the leader's own Wait has collected it.
func forgetLeader(pid int) {
	leaders.mu.Lock()
	defer leaders.mu.Unlock()
	delete(leaders.pids, pid)
}

// buryUnowned collects a dead descendant unless it is a leader whose Wait is
// still to collect it.
func buryUnowned(pid int) {
	leaders.mu.Lock()
	defer leaders.mu.Unlock()
	if leaders.pids[pid] {
		return
	}
	var status syscall.WaitStatus
	_, _ = syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
}

// finalSweep ends whatever is still below this process once every case has
// finished, and names it. Anything found is a process no case accounted for,
// and the run fails on it.
func finalSweep() []string {
	return terminate(nil, "", nil)
}
