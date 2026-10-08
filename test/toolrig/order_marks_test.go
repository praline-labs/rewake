//go:build rewakefixture

package toolrig

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/inbox"
)

// A pending mark between a turn's capture and its journal, rebuilt from
// bridge/server/order_marks_test.go on the fixture's transport: the mark that
// takes the mailbox lock before the journal is the end's, and one that comes
// after it answers not marked. From a tool call's child and from the shell,
// each a process of its own held at one of its durable steps.

// markSteps are the durable steps of a pending mark's process, from a clean
// run: the last one before the mailbox lock, and the mark's own write.
func markSteps(t *testing.T, role string, prepare func(r *rig) int64, run func(r *rig) string) (before, mark int) {
	t.Helper()
	r := newRig(t)
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

// toolMark marks the turn pending through the tool, from the harness
// restarted: the child of a call starts with the plan of its program.
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
				r := newRig(t)
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
