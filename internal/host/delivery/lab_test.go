package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The helpers below are copied from internal/core/mail's tests, which use
// them too: changed only where a name of package mail is qualified.

// readIn has run read a task delivered into a conversation.
func readIn(t *testing.T, dir, run, thread string, m mail.Message) {
	t.Helper()
	if err := state.WithMailboxLock(context.Background(), dir, "api", func() error {
		if err := mailtest.WriteDeliveryThread(dir, "api", m.ID, thread); err != nil {
			return err
		}
		if err := mail.LinkUnread(dir, "api", m.ID); err != nil {
			return err
		}
		return mail.MarkRead(dir, "api", run, m, true)
	}); err != nil {
		t.Fatal(err)
	}
}

func reserve(t *testing.T, dir, question string) func() {
	t.Helper()
	release, err := mail.ReserveAnswer(dir, "api", question)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

// announcedUnread leaves a letter where a serving process leaves one it has
// announced: readable, and no longer waiting.
func announcedUnread(t *testing.T, dir string, message mail.Message) mail.Message {
	t.Helper()
	if err := state.EnsureSubdir(state.InboxPath(dir, message.To)); err != nil {
		t.Fatal(err)
	}
	if err := state.EnsureSubdir(state.UnreadPath(dir, message.To)); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(message)
	if err := os.WriteFile(filepath.Join(state.UnreadPath(dir, message.To), message.ID+".json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return message
}

func writeRaw(t *testing.T, path, raw string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

// pendingTasks writes count tasks from one sender's run, a second apart, that
// wait in the inbox.
func pendingTasks(t *testing.T, dir string, count int) []mail.Message {
	t.Helper()
	var result []mail.Message
	start := time.Now().Add(-time.Minute)
	for index := range count {
		m := message(fmt.Sprintf("task %d", index))
		m.Kind, m.FromEpoch, m.ToEpoch = mail.Task, "sender-epoch", "receiver-epoch"
		m.CreatedAt = start.Add(time.Duration(index) * time.Second)
		if err := mail.Put(dir, m); err != nil {
			t.Fatal(err)
		}
		result = append(result, m)
	}
	return result
}

func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Setenv(state.DirEnv, dir)
	resolved := buildtest.Current(t)
	checkRecordKinds(t, resolved)
	return resolved
}

func message(text string) mail.Message {
	return mail.Message{ID: mail.NewID(), From: "web", To: "api", Text: text, CreatedAt: time.Now()}
}

// endableRun is a run of a name served by a process of its own, which end
// stops and reaps: from then on the run has ended.
type endableRun struct {
	session registry.Session
	child   *exec.Cmd
	once    sync.Once
}

func startRun(t *testing.T, dir, name string) *endableRun {
	t.Helper()
	child := exec.Command("sleep", "60")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	run := &endableRun{child: child, session: registry.Session{Name: name, ServicePID: child.Process.Pid, ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}}
	t.Cleanup(run.end)
	if err := registry.Publish(dir, run.session); err != nil {
		t.Fatal(err)
	}
	return run
}

func (r *endableRun) epoch() string { return r.session.Epoch() }

func (r *endableRun) end() {
	r.once.Do(func() {
		_ = r.child.Process.Kill()
		_ = r.child.Wait()
	})
}

// Every test of this package makes its state directory through stateDir,
// which checks, once the test is over and before the directory goes, that
// every file its writers left in any mailbox is of a kind the list names
// (records.go): a writer that adds a path without adding its kind fails here
// rather than stopping every mailbox its first file lands in.
func checkRecordKinds(t *testing.T, dir string) {
	t.Helper()
	t.Cleanup(func() {
		entries, err := os.ReadDir(state.InboxesPath(dir))
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			unlisted, err := mail.UnlistedRecords(dir, entry.Name())
			if err != nil {
				continue
			}
			for _, path := range unlisted {
				t.Errorf("a writer left %s in the mailbox of %s, which is no kind of record on the list", path, entry.Name())
			}
		}
	})
}

// awaitedBy is what the sender sees of each task it sent, given what became
// of the run it was written for.
func awaitedBy(dir string, what mail.RecipientRun) map[string]mail.AwaitedMessage {
	awaited := map[string]mail.AwaitedMessage{}
	for _, item := range must(mail.Awaited(dir, "web", "sender-epoch", func(string, string) mail.RecipientRun { return what })) {
		awaited[item.ID] = item
	}
	return awaited
}

// twoSessionLab is two sessions, web and a main, lead, both running, and a run of
// api that has ended, whose mailbox holds what the tests leave there: waits it
// owes web and the journals of its turn ends, written as a turn end writes them.
type twoSessionLab struct {
	dir string
	web registry.Session
	// run is api's run: an epoch of this boot whose wrapper is gone.
	run string
}

func newTwoSessionLab(t *testing.T) twoSessionLab {
	t.Helper()
	dir := stateDir(t)
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	var web registry.Session
	for _, name := range []string{"web", "lead"} {
		session := registry.Session{Name: name, ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}
		if name == "lead" {
			session.Role = "main"
		}
		if err := registry.Publish(dir, session); err != nil {
			t.Fatal(err)
		}
		if name == "web" {
			web = session
		}
	}
	return twoSessionLab{dir: dir, web: web, run: registry.RunEpoch(4194000, 7, registrytest.Boot(t))}
}

// report is a report of api's run to web answering tasks.
func (l twoSessionLab) report(kind mail.Kind, tasks ...string) mail.Message {
	return mail.Message{ID: mail.NewID(), From: "api", FromEpoch: l.run, To: "web", ToEpoch: l.web.Epoch(), Kind: kind, Text: "REPORT_TEXT", InReplyTo: tasks, CreatedAt: time.Now()}
}

func (l twoSessionLab) reconcile(t *testing.T) error {
	t.Helper()
	return withLock(l.dir, "api", func() error { return mail.Reconcile(context.Background(), l.dir, "api") })
}

func withLock(dir, name string, fn func() error) error {
	return state.WithMailboxLock(context.Background(), dir, name, fn)
}

// liveRunOf publishes name as a session this process serves, and answers its
// run: a publication to it and a sweep by it find it live.
func liveRunOf(t *testing.T, dir, name string) string {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	session := registry.Session{Name: name, ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: dir, StartedAt: time.Now()}
	if err := registry.Publish(dir, session); err != nil {
		t.Fatal(err)
	}
	return session.Epoch()
}

// must is a lookup's answer in a test that set up nothing it could not read.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

// readAll does what rewake inbox does, under the same lock: look, then mark
// what was looked at. Taking the real lock is the point — a reader that skips
// it passes where rewake inbox would hang.
func readAll(t *testing.T, dir, epoch string) []mail.Message {
	t.Helper()
	var messages []mail.Message
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := state.WithMailboxLock(ctx, dir, "api", func() error {
		var err error
		if messages, err = mail.PeekUnread(dir, "api", epoch); err != nil {
			return err
		}
		for _, m := range messages {
			if err := mail.MarkRead(dir, "api", epoch, m, true); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading under the mailbox lock: %v", err)
	}
	return messages
}

// withdrawNow withdraws under the recipient's mailbox lock, as the command does.
func withdrawNow(t *testing.T, dir string, message mail.Message, replacement *mail.Message) (mail.Withdrawal, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var result mail.Withdrawal
	err := state.WithMailboxLock(ctx, dir, message.To, func() error {
		var err error
		result, err = mail.Withdraw(dir, message, replacement)
		return err
	})
	return result, err
}
