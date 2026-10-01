package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/cli"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// A pending mark between a turn's capture and its journal
// (docs/mail-bridge-checks.md, the order test): the mark that takes the
// mailbox lock before the journal is the end's, and one that comes after it
// answers not marked. From a tool call's child and from the shell, each a
// process of its own held at one of its durable steps.

// hold is the fault build's hold of one process: the directory it signals in.
type hold string

func newHold(t *testing.T) hold { return hold(t.TempDir()) }

func (h hold) spec(role string, step int) string {
	return fmt.Sprintf("%s:hold=%d@%s", role, step, h)
}

// reached waits until the process is held.
func (h hold) reached(t *testing.T) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(string(h), "held")); err == nil {
			return
		}
	}
	t.Fatal("the process was never held")
}

func (h hold) release(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(string(h), "go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// journal publishes the end the gate noted the way the wrapper does.
func (r *rig) journal(boundary *inbox.ReadBoundary, started int64) error {
	return cli.ReportCompletion(context.Background(), r.dir, r.self, harness.Completion{
		Boundary: boundary, ID: "conversation/turn-" + strconv.Itoa(r.turn), Thread: "conversation",
		Kind: inbox.Finished, Text: "the turn's last words", Started: started, Ended: r.endpoint.Gate().Noted(),
	})
}

// shell runs words as api's shell does, with the rig's fault plan.
func (r *rig) shell(words ...string) (string, error) {
	command := exec.Command(binary, words...)
	command.Env = r.env()
	out, err := command.CombinedOutput()
	return string(out), err
}

// report is the kind of the report api's end sent web.
func report(r *rig) inbox.Kind {
	r.t.Helper()
	for _, dir := range []string{state.InboxPath(r.dir, "web"), state.UnreadPath(r.dir, "web")} {
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil || entry.IsDir() {
				continue
			}
			var message inbox.Message
			if json.Unmarshal(raw, &message) == nil && message.From == "api" && message.Kind != inbox.Note {
				return message.Kind
			}
		}
	}
	return ""
}

// markSteps are the durable steps of a pending mark's process, from a clean
// run: the last one before the mailbox lock, and the mark's own write.
func markSteps(t *testing.T, role string, prepare func(r *rig) int64, run func(r *rig) string) (before, mark int) {
	t.Helper()
	r := newRig(t, bridge.CodexTransport)
	prepare(r)
	logPath := filepath.Join(r.root, "fault.log")
	r.fault = role + ":log=" + logPath
	if answer := run(r); !strings.Contains(answer, "marked pending") {
		t.Fatalf("the clean mark: %q", answer)
	}
	steps := durable(parseLog(t, logPath, r.root), role)
	for i, s := range steps {
		if strings.Contains(s.path, "/pending/") {
			return i, i + 1
		}
	}
	t.Fatalf("no mark among %+v", steps)
	return 0, 0
}

// taskRead starts a rig's turn-1 with a task from web read through the tool,
// so its end owes web a report, and returns when the turn started.
func taskRead(r *rig) int64 {
	r.t.Helper()
	r.start()
	started := boottime.Now()
	r.letter("a task from web")
	r.nextTurn()
	read := r.call("inbox")
	if err := r.complete(read, true); err != nil {
		r.t.Fatalf("the read: %v", err)
	}
	return started
}

// toolMark marks the turn pending through the tool, from a fresh server.
func toolMark(r *rig) string {
	r.start()
	return r.call("pending", "the work goes on").result.text()
}

func TestAMarkBetweenCaptureAndJournal(t *testing.T) {
	t.Parallel()
	for _, from := range []struct {
		name, role string
		// mark makes the mark in turn-1 of a rig whose letter from web
		// was read: it returns the answer, and may be started before the
		// test goes on.
		mark func(r *rig) string
	}{
		{"a tool call", "child", toolMark},
		{"the shell", "other", func(r *rig) string {
			out, _ := r.shell("pending", "the work goes on")
			return out
		}},
	} {
		before, inside := markSteps(t, from.role, taskRead, from.mark)
		for _, order := range []struct {
			name   string
			step   int
			marked bool
		}{
			{"the mark holds the lock first", inside, true},
			{"the journal is written first", before, false},
		} {
			t.Run(from.name+"/"+order.name, func(t *testing.T) {
				t.Parallel()
				r := newRig(t, bridge.CodexTransport)
				started := taskRead(r)
				h := newHold(t)
				r.fault = h.spec(from.role, order.step)
				answer := make(chan string, 1)
				go func() { answer <- from.mark(r) }()
				h.reached(t)
				boundary, _ := r.endpoint.Gate().Capture()
				journaled := make(chan error, 1)
				go func() { journaled <- r.journal(boundary, started) }()
				if !order.marked {
					if err := <-journaled; err != nil {
						t.Fatalf("the journal: %v", err)
					}
				}
				h.release(t)
				text := <-answer
				if order.marked {
					if err := <-journaled; err != nil {
						t.Fatalf("the journal: %v", err)
					}
				}
				switch {
				case order.marked && (!strings.Contains(text, "marked pending") || report(r) != inbox.Interim):
					t.Fatalf("a mark before the journal: %q, the end reported %q", text, report(r))
				case !order.marked && (!strings.Contains(text, "not marked") || report(r) != inbox.Finished):
					t.Fatalf("a mark after the journal: %q, the end reported %q", text, report(r))
				}
			})
		}
	}
}
