package harness

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
)

// The holder is the process a probe runs under. It is this same program,
// started again under a name of its own, and it starts nothing but the
// probe's program. As the subreaper of that program's tree, it is the one
// every orphan of the tree comes to; its descendants are therefore exactly
// what the probe started, in whatever group or session they moved to, and
// ending them can never take a process the probe did not start.
//
// It takes its order on fd 3 — the program as one line of JSON, then "end",
// or the end of the pipe when the process that started it is gone — and
// reports on fd 4, one line each: started or no start, the program's exit
// status, and how its tree ended.

// holderName is the argv[0] the holder is started under; no person types it.
const holderName = "rewake-probe-holder"

const (
	orderEnd       = "end"
	reportStarted  = "started"
	reportNoStart  = "nostart"
	reportExit     = "exit"
	reportEnded    = "ended"
	reportLeft     = "left"
	holderOrderFD  = 3
	holderReportFD = 4
)

// holderOrder is the program a holder starts, as exec.Command would.
type holderOrder struct {
	Program string
	Args    []string
	Env     []string
	Dir     string
}

// The holder's entry: before anything of the program it was started from
// runs, so the same binary serves, a test binary included.
func init() {
	if len(os.Args) == 1 && os.Args[0] == holderName {
		// Nothing is buffered to flush, and the exit hooks a test binary
		// may carry would only delay the probe's end.
		syscall.Exit(runHolder())
	}
}

func runHolder() int {
	orders := os.NewFile(holderOrderFD, "order")
	reports := os.NewFile(holderReportFD, "report")
	if orders == nil || reports == nil {
		return 2
	}
	// Neither pipe is the program's: it gets what it would get from the harness.
	syscall.CloseOnExec(holderOrderFD)
	syscall.CloseOnExec(holderReportFD)
	h := &holder{reports: reports}
	end := make(chan struct{})
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	children := make(chan os.Signal, 1)
	signal.Notify(children, syscall.SIGCHLD)
	if setSubreaper() != nil {
		h.report(reportNoStart)
		h.report(reportEnded)
		return 1
	}
	lines := bufio.NewScanner(orders)
	// The order carries the person's environment, which may be long.
	lines.Buffer(nil, 16<<20)
	var order holderOrder
	if !lines.Scan() || json.Unmarshal(lines.Bytes(), &order) != nil || !h.start(order) {
		h.report(reportNoStart)
		h.report(reportEnded)
		return 1
	}
	go func() {
		// "end", anything else, or the pipe closing: the process that
		// started the probe is done with it, or gone.
		lines.Scan()
		close(end)
	}()
	for done := false; !done; {
		select {
		case <-children:
			h.reap()
		case <-signals:
			done = true
		case <-end:
			done = true
		}
	}
	h.endTree()
	return 0
}

type holder struct {
	mu      sync.Mutex
	reports *os.File
	program int
	exited  bool
}

func (h *holder) report(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	_, _ = h.reports.WriteString(line + "\n")
}

// start starts the program in a session of its own, as exec.Command would:
// a name without a slash is looked up on PATH.
func (h *holder) start(order holderOrder) bool {
	program := order.Program
	if !strings.Contains(program, "/") {
		found, err := exec.LookPath(program)
		if err != nil {
			return false
		}
		program = found
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		return false
	}
	defer func() { _ = null.Close() }()
	process, err := os.StartProcess(program, append([]string{order.Program}, order.Args...), &os.ProcAttr{
		Dir: order.Dir, Env: order.Env, Files: []*os.File{null, os.Stdout, os.Stderr},
		// A session of its own: its group is what SIGTERM reaches first.
		Sys: &syscall.SysProcAttr{Setsid: true},
	})
	if err != nil {
		return false
	}
	h.program = process.Pid
	h.report(reportStarted + " " + strconv.Itoa(process.Pid))
	return true
}

// reap reaps every child that has ended, the program's exit reported.
func (h *holder) reap() {
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil || pid <= 0 {
			return
		}
		if pid == h.program && !h.exited {
			h.exited = true
			h.report(reportExit + " " + strconv.FormatUint(uint64(status), 10))
		}
	}
}

// endTree ends the program's group — SIGTERM, then SIGKILL a second later —
// and then every other process of the tree, reaping all of them.
func (h *holder) endTree() {
	pgid := h.program
	if groupAlive(pgid) {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		h.waitGroup(pgid, endGrace)
		if groupAlive(pgid) {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			h.waitGroup(pgid, endGrace)
		}
	}
	self := os.Getpid()
	deadline := time.Now().Add(reapBound)
	for {
		h.reap()
		left := children(self)
		if len(left) == 0 {
			h.report(reportEnded)
			return
		}
		if !time.Now().Before(deadline) {
			h.report(reportLeft + " " + strconv.Itoa(len(left)))
			return
		}
		for _, pid := range left {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func (h *holder) waitGroup(pgid int, bound time.Duration) {
	deadline := time.Now().Add(bound)
	for groupAlive(pgid) && time.Now().Before(deadline) {
		h.reap()
		time.Sleep(20 * time.Millisecond)
	}
	h.reap()
}

// children are root's living children and its zombies. The holder is the
// subreaper of the probe's tree, so a child it kills leaves its own children
// to it: ending its children pass after pass ends the whole tree, and
// nothing outside the tree is ever one of them.
func children(root int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var found []int
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		if parent, _, _, ok := procIDs(pid); ok && parent == root {
			found = append(found, pid)
		}
	}
	return found
}

// procIDs reads a process's parent, group and session from /proc.
func procIDs(pid int) (parent, group, session int, ok bool) {
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0, 0, false
	}
	// The command name may hold spaces and parentheses; the fields
	// follow its last closing parenthesis.
	end := bytes.LastIndexByte(raw, ')')
	if end < 0 {
		return 0, 0, 0, false
	}
	fields := strings.Fields(string(raw[end+1:]))
	if len(fields) < 4 {
		return 0, 0, 0, false
	}
	parent, err1 := strconv.Atoi(fields[1])
	group, err2 := strconv.Atoi(fields[2])
	session, err3 := strconv.Atoi(fields[3])
	return parent, group, session, err1 == nil && err2 == nil && err3 == nil
}

// groupAlive says whether any process of a group is still there, zombies
// that wait to be reaped aside.
func groupAlive(pgid int) bool { return proc.GroupAlive(pgid) }

// setSubreaper makes this process the parent of the orphans of its
// descendants.
func setSubreaper() error {
	const prSetChildSubreaper = 36
	if _, _, errno := syscall.RawSyscall(syscall.SYS_PRCTL, prSetChildSubreaper, 1, 0); errno != 0 {
		return errno
	}
	return nil
}
