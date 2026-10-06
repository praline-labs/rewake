package inbox

import (
	"context"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
	"github.com/praline-labs/rewake/internal/state"
)

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

// owe records that api's run read the tasks from web.
func (l twoSessionLab) owe(t *testing.T, tasks ...string) {
	t.Helper()
	for _, task := range tasks {
		if err := markAwaiting(l.dir, "api", l.run, "web", l.web.Epoch(), task); err != nil {
			t.Fatal(err)
		}
	}
}

// report is a report of api's run to web answering tasks.
func (l twoSessionLab) report(kind Kind, tasks ...string) Message {
	return Message{ID: NewID(), From: "api", FromEpoch: l.run, To: "web", ToEpoch: l.web.Epoch(), Kind: kind, Text: "REPORT_TEXT", InReplyTo: tasks, CreatedAt: time.Now()}
}

func (l twoSessionLab) reconcile(t *testing.T) error {
	t.Helper()
	return withLock(l.dir, "api", func() error { return Reconcile(context.Background(), l.dir, "api") })
}

// copies counts web's letters with id.
func (l twoSessionLab) copies(t *testing.T, id string) int {
	t.Helper()
	letters, err := list(l.dir, "web")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, letter := range letters {
		if letter.ID == id {
			count++
		}
	}
	return count
}

func (l twoSessionLab) owes(t *testing.T, task string) bool {
	t.Helper()
	waiters, err := ReadWaiters(l.dir, "api", l.run)
	if err != nil {
		t.Fatal(err)
	}
	return slices.ContainsFunc(waiters, func(w Waiter) bool { return slices.Contains(w.Messages, task) })
}

// session publishes a running session under name.
func (l twoSessionLab) session(t *testing.T, name string) registry.Session {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	session := registry.Session{Name: name, ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), CWD: l.dir, StartedAt: time.Now()}
	if err := registry.Publish(l.dir, session); err != nil {
		t.Fatal(err)
	}
	return session
}

// journal records an unfinished turn journal of web's under id.
func (l twoSessionLab) journal(t *testing.T, id string, journal TurnJournal) {
	t.Helper()
	journal.Epoch, journal.Op = l.web.Epoch(), id
	if err := WriteJournal(l.dir, "web", id, journal); err != nil {
		t.Fatal(err)
	}
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
