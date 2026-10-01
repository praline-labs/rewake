package cutover

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

const (
	// OutOfSight says the name's session record names a pid namespace other
	// than the launch's, or none: its run cannot be judged from here.
	OutOfSight Kind = iota + EarlierRewake + 1
	// EarlierRun says a run of the earlier build that the name's records
	// name is running, or cannot be told to have ended.
	EarlierRun
)

// NameRecords checks what the name's own records say: its session record's
// namespace, and the wrapper of every earlier-build run they name, each by
// its own pid and start. registry.Session.Alive is not this check: it answers
// for the harness, and says no while the wrapper may still publish.
func NameRecords(dir, name string) []Process {
	var found []Process
	epochs := map[string]bool{}
	session, err := registry.Load(dir, name)
	switch {
	case errors.Is(err, registry.ErrNotFound):
	case err != nil:
		found = append(found, Process{Kind: OutOfSight, Detail: fmt.Sprintf("the session record of %s cannot be read (%v)", name, err)})
	case session.PIDNamespace == "" || session.PIDNamespace != proc.Namespace():
		found = append(found, Process{PID: session.ServicePID, Kind: OutOfSight, Detail: fmt.Sprintf("the session record of %s names no pid namespace, or another than this launch sees, so its run is out of sight", name)})
	default:
		epochs[session.Epoch()] = true
	}
	runs, err := os.ReadDir(state.AwaitingPath(dir, name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		found = append(found, Process{Kind: EarlierRun, Detail: fmt.Sprintf("the waits of %s cannot be listed (%v), so the runs they name are unknown", name, err)})
	}
	for _, run := range runs {
		if !strings.HasPrefix(run.Name(), ".") {
			epochs[run.Name()] = true
		}
	}
	for epoch := range epochs {
		if !registry.EarlierBuildEpoch(epoch) {
			continue
		}
		pid, _, _ := registry.ParseEpoch(epoch)
		switch registry.ObserveRun(epoch) {
		case proc.IdentityAlive:
			found = append(found, Process{PID: pid, Kind: EarlierRun, Detail: fmt.Sprintf("the wrapper of %s's run %s, started by the earlier build, still runs", name, epoch)})
		case proc.IdentityUnknown:
			found = append(found, Process{PID: pid, Kind: EarlierRun, Detail: fmt.Sprintf("whether the wrapper of %s's run %s, started by the earlier build, still runs cannot be read", name, epoch)})
		}
	}
	return found
}

// RefusalError is a launch refused because an earlier-build writer of the
// name is not proven stopped. Nothing is killed: the person stops or waits
// for each process named, and launches again.
type RefusalError struct {
	Name     string
	Blocking []Process
}

var kindWords = map[Kind]string{
	UnknownUser:       "unknown user",
	UnknownExecutable: "unknown executable",
	EarlierRewake:     "earlier rewake",
	OutOfSight:        "out of sight",
	EarlierRun:        "earlier run",
}

func (e *RefusalError) Error() string {
	lines := []string{fmt.Sprintf("rewake was upgraded, and this launch cannot yet prove that no rewake of the earlier build still writes %s's mailbox; stop each process below or wait for it to end, then launch again. Nothing was started or stopped.", e.Name)}
	for _, process := range e.Blocking {
		line := "  " + kindWords[process.Kind] + ": " + process.Detail
		if process.PID > 0 {
			line = fmt.Sprintf("  pid %d, %s: %s", process.PID, kindWords[process.Kind], process.Detail)
		}
		if process.Path != "" {
			line += "; its executable " + process.Path + " is still on disk, a path the upgrade did not replace"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// Check is the whole look for a launch taking name in the room dir, over
// what Look found: nil when every earlier-build writer of the name is proven
// stopped, a *RefusalError naming each one that is not.
func Check(found []Process, dir, name string) error {
	here := Address{Root: state.RootForRoom(dir), Room: filepath.Base(dir), Name: name}
	blocking := append(Blockers(found, here), NameRecords(dir, name)...)
	if len(blocking) == 0 {
		return nil
	}
	return &RefusalError{Name: name, Blocking: blocking}
}
