package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
	"github.com/praline-labs/rewake/internal/state"
)

// The two lock-race schedules of docs/v2/stage3-publication.md#tests for a
// heads-up, through rewake retry of its receipt: the same race as a turn-end
// report's (internal/inbox/publication_race_test.go), on the other path to
// the same section.

// headsUpLab: api's heads-up H to web's run r has landed, the receipt's save
// of it failed, r has read H, and H's age has passed the sweep's cutoff.
type headsUpLab struct {
	dir     string
	self    registry.Session
	web     *exec.Cmd
	r       string
	token   string
	id      string
	mu      sync.Mutex
	letters int
}

// webRun publishes a run of web served by a process of its own, which the
// answered function ends and reaps.
func webRun(t *testing.T, dir string) (registry.Session, *exec.Cmd) {
	t.Helper()
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _, _ = child.Process.Wait() })
	start, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	session := registry.Session{Name: "web", ServicePID: child.Process.Pid, ServiceStart: start, Boot: registrytest.Boot(t), PIDNamespace: proc.Namespace(), CWD: dir, StartedAt: time.Now()}
	_ = os.Remove(state.SessionPath(dir, "web"))
	if err := registry.Publish(dir, session); err != nil {
		t.Fatal(err)
	}
	return session, child
}

var retryLine = regexp.MustCompile(`rewake retry ([0-9a-f]+)`)

func newHeadsUpLab(t *testing.T) *headsUpLab {
	t.Helper()
	dir, self, _ := toolSession(t)
	web, child := webRun(t, dir)
	lab := &headsUpLab{dir: dir, self: self, web: child, r: web.Epoch()}
	receipts := filepath.Join(state.InboxPath(dir, "api"), "receipts") + string(filepath.Separator)
	mailbox := state.InboxPath(dir, "web") + string(filepath.Separator)
	// Every save of the receipt after the landing fails until the send has
	// answered: a later save would carry Published from memory.
	failing, failed := true, false
	state.Fault = func(op, path string) error {
		if op != state.OpWrite {
			return nil
		}
		lab.mu.Lock()
		defer lab.mu.Unlock()
		switch {
		case strings.HasPrefix(path, mailbox) && strings.HasSuffix(path, ".json") && !strings.Contains(path, string(filepath.Separator)+"receipts"+string(filepath.Separator)):
			lab.letters++
		case lab.letters > 0 && failing && strings.HasPrefix(path, receipts) && strings.HasSuffix(path, ".json"):
			failed = true
			return &os.PathError{Op: "write", Path: path, Err: syscall.EIO}
		}
		return nil
	}
	t.Cleanup(func() { state.Fault = nil })
	sent := newToolCaller(t).run("send", "web", "look", "--notify", "--wait", "0")
	lab.mu.Lock()
	failing = false
	lab.mu.Unlock()
	token := retryLine.FindStringSubmatch(sent.out + sent.errOut)
	if !failed || token == nil {
		t.Fatalf("the heads-up's save did not fail after it landed: %+v", sent)
	}
	lab.token = token[1]
	record, err := receipt.Load(dir, "api", self.Epoch(), lab.token)
	if err != nil || record.Notify == nil || record.Notify.Published {
		t.Fatalf("the receipt: %+v %v", record, err)
	}
	lab.id = record.Notify.MessageID
	read := filepath.Join(state.DonePath(dir, "web"), lab.id+".json")
	if err := state.EnsureSubdir(state.DonePath(dir, "web")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(state.InboxPath(dir, "web"), lab.id+".json"), read); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(read, old, old); err != nil {
		t.Fatal(err)
	}
	return lab
}

// nextRunSweeps ends r and serves web's next run until its first sweep has
// finished; the answer is closed then.
func (l *headsUpLab) nextRunSweeps(t *testing.T) <-chan struct{} {
	t.Helper()
	_ = l.web.Process.Kill()
	_, _ = l.web.Process.Wait()
	next, _ := webRun(t, l.dir)
	ctx, cancel := context.WithCancel(context.Background())
	swept, served := make(chan struct{}), make(chan struct{})
	server := &inbox.Server{Dir: l.dir, Name: "web", Epoch: next.Epoch(), Ready: func() { close(swept) }}
	go func() {
		defer close(served)
		server.Serve(ctx)
	}()
	t.Cleanup(func() { cancel(); <-served })
	return swept
}

// finished waits, bounded, for the sweep a schedule started.
func (l *headsUpLab) finished(t *testing.T, swept <-chan struct{}) {
	t.Helper()
	if swept == nil {
		t.Fatal("the publication never reached the schedule's step")
	}
	select {
	case <-swept:
	case <-time.After(raceBound):
		t.Fatal("the sweep did not finish")
	}
}

func (l *headsUpLab) writes() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.letters
}

func (l *headsUpLab) proofGone(t *testing.T) bool {
	t.Helper()
	_, letter := os.Stat(filepath.Join(state.DonePath(l.dir, "web"), l.id+".json"))
	_, marks := os.Stat(filepath.Join(state.InboxPath(l.dir, "web"), "once", l.r))
	return os.IsNotExist(letter) && os.IsNotExist(marks)
}

// retry runs rewake retry of H and answers once its receipt is published.
// What the retry answers past that is H's delivery: r has ended without a
// status for it, which the send reports as failed for web.
func (l *headsUpLab) retry(t *testing.T) {
	t.Helper()
	if code, out, errOut := run("retry", l.token); code == ExitUsage || strings.Contains(out+errOut, "not published") || strings.Contains(out+errOut, "could not write") {
		t.Fatalf("retry: %d %s %s", code, out, errOut)
	}
	record, err := receipt.Load(l.dir, "api", l.self.Epoch(), l.token)
	if err != nil || record.Notify == nil || !record.Notify.Published {
		t.Fatalf("the receipt is not marked published: %+v %v", record, err)
	}
}

// at runs fn, once, when the publication of H reaches step; and answers once
// somebody finds web's mailbox lock held.
func (l *headsUpLab) at(t *testing.T, step string, fn func()) <-chan struct{} {
	t.Helper()
	var once, waited sync.Once
	inbox.PublicationStep = func(at string, message inbox.Message) {
		if at == step && message.ID == l.id {
			once.Do(fn)
		}
	}
	waiting, mailbox := make(chan struct{}), state.InboxPath(l.dir, "web")
	state.LockWait = func(path string) {
		if path == mailbox {
			waited.Do(func() { close(waiting) })
		}
	}
	t.Cleanup(func() { inbox.PublicationStep, state.LockWait = nil, nil })
	return waiting
}

const raceBound = 10 * time.Second

// The sweep waits for an attempt's evidence, for a heads-up: the retry is at
// its first evidence read, inside the section, when r ends and the next
// run's sweep starts. The sweep waits; the retry finds the letter, writes
// nothing and marks the receipt published.
func TestTheSweepWaitsForAHeadsUpsEvidence(t *testing.T) {
	lab := newHeadsUpLab(t)
	var swept <-chan struct{}
	var waiting <-chan struct{}
	waiting = lab.at(t, inbox.PublicationEvidence, func() {
		swept = lab.nextRunSweeps(t)
		select {
		case <-waiting:
		case <-swept:
			t.Error("the sweep ran inside the publisher's section")
		case <-time.After(raceBound):
			t.Error("the sweep neither waited nor finished")
		}
	})
	lab.retry(t)
	lab.finished(t, swept)
	if writes := lab.writes(); writes != 1 {
		t.Fatalf("H was written %d times", writes)
	}
	if !lab.proofGone(t) {
		t.Fatal("the sweep after the section retired nothing")
	}
}

// Admission is inside the section, for a heads-up: the retry held at its
// successful liveness read holds web's lock, so the next run's sweep waits;
// the retry finds the letter and writes nothing, and the sweep then retires
// the proof.
func TestAHeadsUpsAdmissionIsInsideTheSection(t *testing.T) {
	lab := newHeadsUpLab(t)
	var swept <-chan struct{}
	var waiting <-chan struct{}
	waiting = lab.at(t, inbox.PublicationAdmitted, func() {
		swept = lab.nextRunSweeps(t)
		select {
		case <-waiting:
		case <-swept:
		case <-time.After(raceBound):
			t.Error("the sweep neither waited nor finished")
		}
	})
	lab.retry(t)
	lab.finished(t, swept)
	if writes := lab.writes(); writes != 1 {
		t.Fatalf("H was written %d times", writes)
	}
	if !lab.proofGone(t) {
		t.Fatal("the sweep after the section retired nothing")
	}
}
