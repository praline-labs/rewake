package mail

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
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

// report is a report of api's run to web answering tasks.
func (l twoSessionLab) report(kind Kind, tasks ...string) Message {
	return Message{ID: NewID(), From: "api", FromEpoch: l.run, To: "web", ToEpoch: l.web.Epoch(), Kind: kind, Text: "REPORT_TEXT", InReplyTo: tasks, CreatedAt: time.Now()}
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
