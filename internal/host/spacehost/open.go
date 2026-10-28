package spacehost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/space"
	hostlaunch "github.com/praline-labs/rewake/internal/host/launch"
	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// openBound is how long an open waits for the worker's session to register:
// its wrapper registers before the harness is up, within seconds. A test
// shortens it.
var openBound = 30 * time.Second

// workerSize is the worker's terminal until the shell shows it: the shell
// sizes it to its pane.
var workerSize = terminal.Size{Cols: 120, Rows: 40}

// startHost is Start; a test of the open's other steps replaces it.
var startHost = Start

// Opening is a space main asks to open: its name and the worker raised for
// it, in main's room.
type Opening struct {
	// Dir is main's room.
	Dir string
	// Name is the space's; the worker's session is <Name>-<harness>.
	Name string
	// Main and Run are main's session and its run, which the space lives no
	// longer than.
	Main, Run string
	Worker    hostlaunch.Raised
	Catalog   adapter.Catalog
	// Dev raises the worker drawn by rewake's own face (docs/face.md).
	Dev bool
}

// Opened is a space whose worker registered.
type Opened struct {
	// Record is the space's record as the open last read it: open, the
	// worker's run on it.
	Record space.Record
	// Session is the worker's session.
	Session string
	// Notice is what the worker still lacks that the person gives in its
	// pane — a login of its own —, for main to tell the person.
	Notice string
}

// RequestError refuses an open for what it asks — a name that cannot be one,
// a launch the worker's harness would refuse — before anything is written.
type RequestError struct{ Reason string }

func (e *RequestError) Error() string { return e.Reason }

// openFailure is an open that failed, its text the whole sentence main is
// shown.
type openFailure struct{ reason string }

func (f *openFailure) Error() string { return f.reason }

func failure(format string, args ...any) error {
	return &openFailure{fmt.Sprintf(format, args...)}
}

// Open opens a space (docs/rules/space.md): it writes the record, starts the
// detached space host, and waits for the worker's session to register under
// the run the host recorded. A space host that cannot be started, or never
// records itself, leaves the record closed with its worker never started.
func Open(o Opening) (Opened, error) {
	h := o.Worker.Harness
	// Asked before the worker starts: the harness's own status command may
	// write the state the worker's harness then reads.
	var notice string
	if ready, ok := h.(adapter.WorkerReadiness); ok {
		notice = ready.WorkerNotice(o.Worker.Command)
	}
	session := o.Name + "-" + h.ID()
	if _, ok := h.(adapter.MirrorHarness); o.Dev && !ok {
		return Opened{}, &RequestError{fmt.Sprintf("--dev, yet a %s session cannot be drawn by rewake's own face.", h.Title())}
	}
	if !state.ValidName(o.Name) || !state.ValidName(session) {
		return Opened{}, &RequestError{fmt.Sprintf("%q cannot name a space whose worker is %s: lower-case letters, digits, dot, dash or underscore, the session 32 characters at most.", o.Name, session)}
	}
	if other, err := registry.LookupReadOnly(o.Dir, session); err == nil && other.Alive() {
		return Opened{}, failure("the session %s already runs in this room; choose another name for the space.", session)
	}
	record := space.Record{
		Name:     o.Name,
		Main:     space.Run{Name: o.Main, Run: o.Run, PIDNamespace: proc.Namespace()},
		Worker:   space.Worker{Harness: h.ID(), Name: o.Name, Role: o.Worker.Role.ID, Type: o.Worker.Work.Name, Label: o.Worker.Label, Dev: o.Dev},
		OpenedAt: time.Now().UTC(),
	}
	var err error
	if o.Catalog.Offers(h.ID()).Conversation.Names() {
		if record.Worker.SessionID, err = space.NewSessionID(); err != nil {
			return Opened{}, err
		}
	}
	if record.ID, err = space.NewID(); err != nil {
		return Opened{}, err
	}
	words, err := o.Worker.Launch(record.Worker.SessionID)
	if err != nil {
		return Opened{}, &RequestError{err.Error()}
	}
	spec, err := hostSpec(o, record.ID, words)
	if err != nil {
		return Opened{}, err
	}
	var taken *space.NameTakenError
	if err := space.Create(o.Dir, record); errors.As(err, &taken) {
		return Opened{}, err
	} else if err != nil {
		return Opened{}, fmt.Errorf("could not write the space's record: %w", err)
	}
	if err := startHost(spec); err != nil {
		closeUnstarted(o.Dir, record.ID, err.Error())
		return Opened{}, fmt.Errorf("the space %s did not open: %w", o.Name, err)
	}
	opened, err := awaitWorker(o.Dir, record.ID, session)
	if err != nil {
		return Opened{}, err
	}
	return Opened{Record: opened, Session: session, Notice: notice}, nil
}

// hostSpec is the space host's spec: main's room and working directory, the
// worker's launch, every harness's markers for the worker not to inherit,
// and what the worker's launch is told of its template.
func hostSpec(o Opening, id string, words []string) (Spec, error) {
	base, err := state.BaseForRoom(o.Dir)
	if err != nil {
		return Spec{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return Spec{}, err
	}
	var strip []string
	for _, h := range o.Catalog.All() {
		strip = append(strip, h.EnvNotInherited()...)
	}
	return Spec{
		Base: base, Room: filepath.Base(o.Dir), Dir: o.Dir, ID: id, Args: words, Cwd: cwd, Strip: strip, Size: workerSize,
		Template: o.Worker.Template, Worktree: o.Worker.Worktree != "", Enter: o.Worker.Enter,
		Type: o.Worker.Work.Name, TypeText: o.Worker.Work.Text,
	}, nil
}

// awaitWorker waits until the worker's session registered under the run the
// space host recorded, or the space closed, saying why.
func awaitWorker(dir, id, session string) (space.Record, error) {
	deadline := time.Now().Add(openBound)
	for {
		record, err := space.Load(dir, id)
		if err != nil {
			return space.Record{}, fmt.Errorf("could not read the space's record: %w", err)
		}
		if record.State.Final() {
			return space.Record{}, fmt.Errorf("the space %s did not open: %s", record.Name, record.End.Why)
		}
		if record.State == space.Open {
			if worker, err := registry.LookupReadOnly(dir, session); err == nil && worker.Epoch() == record.Worker.Run {
				return record, nil
			}
		}
		if time.Now().After(deadline) {
			if record.Host == "" {
				closeUnstarted(dir, id, fmt.Sprintf("the space host did not record itself within %s", openBound))
				return space.Record{}, failure("the space %s did not open: its space host did not start within %s.", record.Name, openBound)
			}
			return space.Record{}, failure("the worker %s has not registered within %s; the space is still %s: rewake space list shows it, and main in the shell shows its terminal.", session, openBound, record.State)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// closeUnstarted closes the record of a space whose worker never started,
// from what the open saw.
func closeUnstarted(dir, id, why string) {
	_, _ = space.Close(dir, id, space.End{At: time.Now().UTC(), Why: why, Worker: space.WorkerNeverStarted, Checkout: space.Seen{Presence: space.Gone}})
}
