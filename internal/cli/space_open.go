package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// openBound is how long rewake space open waits for the worker's session to
// register: its wrapper registers before the harness is up, within seconds.
const openBound = 30 * time.Second

// workerSize is the worker's terminal until the shell shows it: the shell
// sizes it to its pane.
var workerSize = terminal.Size{Cols: 120, Rows: 40}

// spaceOpened is what rewake space open answers.
type spaceOpened struct {
	Space     string `json:"space"`
	ID        string `json:"id"`
	Session   string `json:"session"`
	Harness   string `json:"harness"`
	Role      string `json:"role"`
	Room      string `json:"room"`
	SessionID string `json:"sessionId,omitempty"`
	Worktree  bool   `json:"worktree,omitempty"`
}

func (o spaceOpened) lines() []string {
	return []string{
		fmt.Sprintf("Rewake: opened space %s: %s (%s, %s) runs in room %s and closes when this run ends or on rewake space close %s.", o.Space, o.Session, o.Harness, o.Role, o.Room, o.Space),
		fmt.Sprintf("Give it a task with rewake send %s \"...\"; main in the shell shows its terminal as the worker pane the bar selects.", o.Session),
	}
}

// handleSpaceOpen starts a worker in a space of main's run: it writes the
// record, starts the detached space host, and waits for the worker's session
// to register (docs/rules/space.md).
func handleSpaceOpen(ctx *Context, call Call) error {
	usage := func(format string, args ...any) error {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	if len(call.Positionals) < 2 {
		return usage("space open needs the space's name: rewake space open api --alias <alias>.")
	}
	name := call.Positionals[1]
	dir, err := state.Dir()
	if err != nil {
		return usage("%s", err.Error())
	}
	self, epoch, err := ownRun(dir)
	if errors.Is(err, errNotASession) {
		return usage("only main opens a space, from inside its own session; this shell runs in none.")
	}
	if err != nil {
		return failf("%v", err)
	}
	catalog := ctx.table.catalog
	if err := spacesRefusal(call.Command, catalog, self); err != nil {
		return err
	}
	target, err := ctx.spaceTarget(call)
	if err != nil {
		return err
	}
	h, part := target.harness, target.part
	if err := refuseReportingRole(h, part); err != nil {
		return err
	}
	session := name + "-" + h.ID()
	if !state.ValidName(name) || !state.ValidName(session) {
		return usage("%q cannot name a space whose worker is %s: lower-case letters, digits, dot, dash or underscore, the session 32 characters at most.", name, session)
	}
	if other, err := registry.LookupReadOnly(dir, session); err == nil && other.Alive() {
		return failf("the session %s already runs in this room; choose another name for the space.", session)
	}
	record := space.Record{
		Name:     name,
		Main:     space.Run{Name: self.Name, Run: epoch, PIDNamespace: proc.Namespace()},
		Worker:   space.Worker{Harness: h.ID(), Name: name, Role: part.ID},
		OpenedAt: time.Now().UTC(),
	}
	if conversation := catalog.Offers(h.ID()).Conversation; conversation.Names() {
		if record.Worker.SessionID, err = space.NewSessionID(); err != nil {
			return failf("%v", err)
		}
	}
	if record.ID, err = space.NewID(); err != nil {
		return failf("%v", err)
	}
	launchWords, err := workerLaunch(target, call, record.Worker.SessionID)
	if err != nil {
		return err
	}
	spec, err := hostSpec(ctx.table.catalog, dir, record.ID, launchWords)
	if err != nil {
		return failf("%v", err)
	}
	spec.Template = target.template
	spec.Worktree = target.worktree != ""
	var taken *space.NameTakenError
	if err := space.Create(dir, record); errors.As(err, &taken) {
		return failf("%v", err)
	} else if err != nil {
		return failf("could not write the space's record: %v", err)
	}
	if err := spacehost.Start(spec); err != nil {
		closeUnstarted(dir, record.ID, err.Error())
		return failf("the space %s did not open: %v", name, err)
	}
	if err := awaitWorker(dir, record.ID, session); err != nil {
		return err
	}
	opened := spaceOpened{Space: name, ID: record.ID, Session: session, Harness: h.ID(), Role: part.ID, Room: filepath.Base(dir), SessionID: record.Worker.SessionID, Worktree: spec.Worktree}
	return printValue(ctx, opened, opened.lines)
}

// hostSpec is the space host's spec: main's room and working directory, the
// worker's launch, and every harness's markers for the worker not to inherit.
func hostSpec(catalog adapter.Catalog, dir, id string, launch []string) (spacehost.Spec, error) {
	base, err := state.BaseForRoom(dir)
	if err != nil {
		return spacehost.Spec{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return spacehost.Spec{}, err
	}
	var strip []string
	for _, h := range catalog.All() {
		strip = append(strip, h.EnvNotInherited()...)
	}
	return spacehost.Spec{Base: base, Room: filepath.Base(dir), Dir: dir, ID: id, Args: launch, Cwd: cwd, Strip: strip, Size: workerSize}, nil
}

// awaitWorker waits until the worker's session registered under the run the
// space host recorded, or the space closed, saying why.
func awaitWorker(dir, id, session string) error {
	deadline := time.Now().Add(openBound)
	for {
		record, err := space.Load(dir, id)
		if err != nil {
			return failf("could not read the space's record: %v", err)
		}
		if record.State.Final() {
			return failf("the space %s did not open: %s", record.Name, record.End.Why)
		}
		if record.State == space.Open {
			if worker, err := registry.LookupReadOnly(dir, session); err == nil && worker.Epoch() == record.Worker.Run {
				return nil
			}
		}
		if time.Now().After(deadline) {
			if record.Host == "" {
				closeUnstarted(dir, id, fmt.Sprintf("the space host did not record itself within %s", openBound))
				return failf("the space %s did not open: its space host did not start within %s.", record.Name, openBound)
			}
			return failf("the worker %s has not registered within %s; the space is still %s: rewake space list shows it, and main in the shell shows its terminal.", session, openBound, record.State)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// closeUnstarted closes the record of a space whose worker never started,
// from what the open saw.
func closeUnstarted(dir, id, why string) {
	_, _ = space.Close(dir, id, space.End{At: time.Now().UTC(), Why: why, Worker: space.WorkerNeverStarted, Checkout: space.Seen{Presence: space.Gone}})
}
