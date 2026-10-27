package spacehost

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/registry/registrytest"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/space"
	hostlaunch "github.com/praline-labs/rewake/internal/host/launch"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

type openHarness struct{ adapter.Harness }

func (openHarness) ID() string                { return "agent" }
func (openHarness) Title() string             { return "Test agent" }
func (openHarness) Offers() adapter.Offer     { return adapter.Offer{} }
func (openHarness) EnvNotInherited() []string { return nil }

// opening is a space of a room of the test's own, main's run and the
// worker's both this process, and the space host stood in for by host,
// which is given the room and the spec the open would start it with.
func opening(t *testing.T, host func(dir string, spec Spec) error) Opening {
	t.Helper()
	dir := buildtest.Dir(t)
	self, err := selfRun()
	if err != nil {
		t.Fatal(err)
	}
	startHost = func(spec Spec) error { return host(dir, spec) }
	t.Cleanup(func() { startHost = Start })
	return Opening{
		Dir: dir, Name: "api", Main: "main-agent", Run: self,
		Worker:  hostlaunch.Raised{Harness: openHarness{}, Role: role.General, Template: "w"},
		Catalog: adapter.NewCatalog(openHarness{}),
	}
}

// registered publishes the worker's session as this process's run.
func registered(t *testing.T, dir string) error {
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		return err
	}
	return registry.Publish(dir, registry.Session{
		Name: "api-agent", Role: role.General.ID, Harness: "agent",
		ServicePID: os.Getpid(), ServiceStart: start, Boot: registrytest.Boot(t), PIDNamespace: proc.Namespace(),
	})
}

// The open answers once the space host has opened the record and the
// worker's session registered under the run the host recorded, with the
// spec the host was started with carrying the worker's launch and template.
func TestAnOpenAnswersOnceTheWorkerRegistered(t *testing.T) {
	var started Spec
	o := opening(t, func(dir string, spec Spec) error {
		started = spec
		self, err := selfRun()
		if err != nil {
			return err
		}
		if _, err := space.SetHost(dir, spec.ID, self); err != nil {
			return err
		}
		if _, err := space.Opened(dir, spec.ID, self); err != nil {
			return err
		}
		go func() {
			time.Sleep(200 * time.Millisecond)
			_ = registered(t, dir)
		}()
		return nil
	})
	opened, err := Open(o)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Session != "api-agent" || opened.Record.State != space.Open || opened.Record.ID != started.ID || opened.Record.Main.Run != o.Run {
		t.Fatalf("opened %+v", opened)
	}
	if started.Template != "w" || len(started.Args) == 0 || started.Args[0] != "agent" || started.Dir != o.Dir {
		t.Fatalf("the host was started with %+v", started)
	}
}

// An open refused for its name, or for a session of that name already
// running, writes nothing; one whose space host does not start, never
// records itself or closes the space answers why, and leaves the record
// closed with the worker never started.
func TestAnOpenThatFailsLeavesNoOpenSpace(t *testing.T) {
	defer func(bound time.Duration) { openBound = bound }(openBound)
	openBound = 500 * time.Millisecond
	for _, c := range []struct {
		name   string
		host   func(dir string, spec Spec) error
		before func(o *Opening) error
		answer string
		why    string
		// wrong says the call was wrong, not the open.
		wrong bool
	}{
		{name: "a name that cannot be one", before: func(o *Opening) error { o.Name = "API"; return nil }, answer: `"API" cannot name a space`, wrong: true},
		{name: "a session of that name", before: func(o *Opening) error { return registered(t, o.Dir) }, answer: "the session api-agent already runs in this room"},
		{
			name:   "no space host",
			host:   func(string, Spec) error { return errors.New("the launcher failed") },
			answer: "the space api did not open: the launcher failed", why: "the launcher failed",
		},
		{
			name:   "a space host that never records itself",
			host:   func(string, Spec) error { return nil },
			answer: "its space host did not start within", why: "the space host did not record itself within",
		},
		{
			name: "a space host that refuses",
			host: func(dir string, spec Spec) error {
				if _, err := space.SetHost(dir, spec.ID, "1.1"); err != nil {
					return err
				}
				closeUnstarted(dir, spec.ID, "its host runs below main")
				return nil
			},
			answer: "the space api did not open: its host runs below main", why: "its host runs below main",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			var id string
			o := opening(t, func(dir string, spec Spec) error {
				id = spec.ID
				return c.host(dir, spec)
			})
			if c.before != nil {
				if err := c.before(&o); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Open(o)
			if err == nil || !strings.Contains(err.Error(), c.answer) {
				t.Fatalf("answered %v, want %q", err, c.answer)
			}
			var wrong *RequestError
			if errors.As(err, &wrong) != c.wrong {
				t.Errorf("%v: a wrong request %v, want %v", err, wrong != nil, c.wrong)
			}
			records, _, err := space.List(o.Dir)
			if err != nil {
				t.Fatal(err)
			}
			if c.host == nil {
				if len(records) != 0 {
					t.Fatalf("a refused open wrote %+v", records)
				}
				return
			}
			record, err := space.Load(o.Dir, id)
			if err != nil {
				t.Fatal(err)
			}
			if record.State != space.Closed || record.End.Worker != space.WorkerNeverStarted || !strings.HasPrefix(record.End.Why, c.why) {
				t.Fatalf("left %s with %+v", record.State, record.End)
			}
		})
	}
}
