package workflow

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// reexecRecord is the part of a session's record the re-exec probe compares
// before and after the exec.
type reexecRecord struct {
	ServicePID   int    `json:"servicePid"`
	ServiceStart uint64 `json:"serviceStart"`
	Boot         string `json:"boot"`
	HarnessPID   int    `json:"harnessPid"`
	HarnessStart uint64 `json:"harnessStart"`
	Socket       string `json:"socket"`
	path         string
}

func (r reexecRecord) epoch() string {
	return strconv.Itoa(r.ServicePID) + "." + strconv.FormatUint(r.ServiceStart, 10) + "." + r.Boot
}

// reexecRecordOf reads a session's record from the case's state directory.
func reexecRecordOf(iso *Isolation, name string) reexecRecord {
	paths, _ := filepath.Glob(filepath.Join(iso.StateDir, "v2", "rooms", "*", "sessions", name+".json"))
	var record reexecRecord
	if len(paths) == 1 {
		raw, _ := os.ReadFile(paths[0])
		_ = json.Unmarshal(raw, &record)
		record.path = paths[0]
	}
	return record
}

// childrenOf lists a process's children, from every one of its threads: a
// child belongs to the thread that forked it.
func childrenOf(pid int) []int {
	lists, _ := filepath.Glob(filepath.Join("/proc", strconv.Itoa(pid), "task", "*", "children"))
	var children []int
	for _, list := range lists {
		raw, _ := os.ReadFile(list)
		for _, field := range strings.Fields(string(raw)) {
			if child, err := strconv.Atoi(field); err == nil {
				children = append(children, child)
			}
		}
	}
	return children
}

// alive says a process runs and is not a zombie.
func alive(pid int) bool {
	stat, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	_, after, _ := strings.Cut(string(stat), ") ")
	return !strings.HasPrefix(after, "Z")
}

func allAlive(pids []int) bool {
	for _, pid := range pids {
		if !alive(pid) {
			return false
		}
	}
	return len(pids) > 0
}

func anyAlive(pids []int) bool {
	for _, pid := range pids {
		if alive(pid) {
			return true
		}
	}
	return false
}

// signalReexec asks a wrapper to re-execute and answers what it wrote of the
// outcome, empty when it wrote nothing in time.
func signalReexec(c *Case, pid int, mark string) string {
	_ = os.Remove(mark)
	if err := syscall.Kill(pid, syscall.SIGUSR2); err != nil {
		return "not signaled: " + err.Error()
	}
	var outcome string
	waitFor(c, 15*time.Second, func() bool {
		raw, err := os.ReadFile(mark)
		outcome = strings.TrimSpace(string(raw))
		return err == nil && outcome != ""
	})
	return outcome
}

// replaced says the process runs an image an exec handed a run over to: the
// environment it was executed with carries the handover.
func replaced(pid int) bool {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
	environ := "\x00" + string(raw)
	return err == nil && strings.Contains(environ, "\x00REWAKE_HANDOVER=")
}

// dials says a unix socket takes a connection.
func dials(path string) bool {
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// distinctReplies counts the tasks a session's finished reports answer.
func distinctReplies(reader, about *scenarioSession) int {
	answered := map[string]bool{}
	for _, message := range readMessages(reader) {
		if message.From == about.name && message.Kind == "finished" {
			for _, id := range message.InReplyTo {
				answered[id] = true
			}
		}
	}
	return len(answered)
}
