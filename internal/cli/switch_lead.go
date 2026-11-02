package cli

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/switching"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The deferred switch (docs/rules/state.md#switching-a-running-room): main
// asks for it from its own turn, which keeps main's wrapper busy for as long
// as the command would wait. So the command hands the switch to a leader in a
// process of its own, bound to the main that runs when it asked; the leader
// tries again every deferPause while an attempt ends only on a busy session,
// for deferFor at most, and never against another run of main.

const (
	// switchLeadEnv carries a leader's request; switchOutcomeEnv the outcome
	// its last image delivers (switch_outcome.go).
	switchLeadEnv    = "REWAKE_SWITCH_LEAD"
	switchOutcomeEnv = "REWAKE_SWITCH_OUTCOME"
	// The descriptors a leader inherits: the request's held file, and the
	// pipe a waiting command reads the outcome from.
	requestFD = 3
	pipeFD    = 4
)

// deferFor bounds a deferred switch from its asking, and deferPause is the
// wait before another attempt.
var (
	deferFor   = 10 * time.Minute
	deferPause = 5 * time.Second
)

// leadSpec is one deferred switch's request.
type leadSpec struct {
	Base string `json:"base"`
	Room string `json:"room"`
	// Main and MainEpoch are main's run when the switch was asked; empty in
	// a room with no main registered.
	Main      string `json:"main,omitempty"`
	MainEpoch string `json:"mainEpoch,omitempty"`
	// Asker is the session that asked, "" for the person.
	Asker      string `json:"asker,omitempty"`
	AskerEpoch string `json:"askerEpoch,omitempty"`
	From       string `json:"from"`
	Asked      int64  `json:"asked"`
	// Installed is the install the command read, whose rewake each attempt
	// stages anew.
	Installed string `json:"installed"`
	// For and Pause are deferFor and deferPause as the command had them.
	For    time.Duration    `json:"for"`
	Pause  time.Duration    `json:"pause"`
	Pipe   bool             `json:"pipe,omitempty"`
	Bounds switching.Bounds `json:"bounds"`
}

// roomMain is main's run in the room's registry, by name and epoch.
func roomMain(dir string) (string, string) {
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return "", ""
	}
	for _, s := range sessions {
		if s.Role == role.Main.ID {
			return s.Name, s.Epoch()
		}
	}
	return "", ""
}

// mainGone says why the run the switch was asked for no longer runs, or "".
func (s leadSpec) mainGone() string {
	if s.Main == "" {
		return ""
	}
	current, err := registry.LookupReadOnly(s.Room, s.Main)
	if errors.Is(err, registry.ErrNotFound) || err == nil && current.Epoch() != s.MainEpoch {
		return "main " + s.Main + " ended before the switch could run; nothing was switched"
	}
	return ""
}

// startLeader starts the leader in a session of its own, which the end of
// the harness's tool call does not take, holding the request and the pipe.
func startLeader(spec leadSpec, request, pipe *os.File) error {
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		return err
	}
	defer func() { _ = devnull.Close() }()
	// The room's build: this process runs it, whatever the install holds.
	cmd := exec.Command("/proc/self/exe")
	cmd.Env = append(withoutEnv(os.Environ(), switchLeadEnv, switchOutcomeEnv), switchLeadEnv+"="+string(raw))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	cmd.ExtraFiles = []*os.File{request}
	if pipe != nil {
		cmd.ExtraFiles = append(cmd.ExtraFiles, pipe)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// leadSwitch is the leader's process: attempts until one is not refused as
// busy, the bound passes or main's run ends, then the outcome delivered by
// the room's build.
func leadSwitch(catalog adapter.Catalog, raw string) int {
	// Its own session has no terminal; a signal of the person's is not its.
	signal.Ignore(syscall.SIGHUP, syscall.SIGINT)
	var spec leadSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return ExitFailed
	}
	var pipe *os.File
	if spec.Pipe {
		pipe = os.NewFile(pipeFD, "switch-outcome")
	}
	until := time.UnixMilli(spec.Asked).Add(spec.For)
	var out switchOutcome
	for {
		if why := spec.mainGone(); why != "" {
			out = switchOutcome{Fail: why, Gone: true}
			break
		}
		var busy bool
		out, busy = attempt(catalog, spec)
		last := time.Now().Add(spec.Pause).After(until)
		if busy && last {
			out.Fail += "; tried again for " + spec.For.String() + " since it was asked"
		}
		if !busy || last {
			break
		}
		time.Sleep(spec.Pause)
	}
	return deliver(spec, out, pipe)
}

// attempt is one claim of the room, the first stage's flow, under a lease of
// its own let go at its end: a leader waiting between attempts holds nothing
// a launch meets. busy says it may be tried again.
func attempt(catalog adapter.Catalog, spec leadSpec) (switchOutcome, bool) {
	refused := func(why string) switchOutcome {
		return switchOutcome{Fail: "rewake was not switched; the room stays on build " + build.Short(spec.From) + ": " + why}
	}
	lease, err := build.Acquire(spec.Base, filepath.Base(spec.Room))
	if err != nil {
		return refused(err.Error()), false
	}
	defer lease.Release()
	installed := filepath.Join(spec.Installed, "rewake")
	target, err := build.FileID(installed)
	if err != nil {
		return refused("the installed rewake cannot be read: " + err.Error()), false
	}
	if target == spec.From {
		return switchOutcome{View: switchView{From: spec.From, To: target, Adopted: []string{}, Lost: []string{}}}, false
	}
	b := spec.Bounds
	sw, err := build.Claim(lease, target, state.SocketPath(spec.Room, "switch", ""), time.Now().Add(b.Settle+b.Ask+b.Drop))
	if err != nil {
		return refused(err.Error()), false
	}
	if err := sw.Stage(installed); err != nil {
		_ = sw.End()
		return refused("the installed rewake could not be staged: " + err.Error()), false
	}
	if why := admits(catalog, sw.Mark.Staged); why != "" {
		_ = sw.End()
		return refused(why + "; restart the room — end main and start it again — to load it"), false
	}
	o := switching.LeadUntil(sw, b, spec.mainGone)
	view := switchView{From: spec.From, To: target, Switched: o.Switched, Adopted: nonNil(o.Adopted), Lost: nonNil(o.Lost)}
	switch {
	case !o.Switched:
		return refused(o.Why), o.Busy
	case len(o.Lost) > 0:
		return switchOutcome{View: view, Fail: "the room switched to build " + build.Short(target) + ", and " + o.Why}, false
	}
	return switchOutcome{View: view}, false
}

// withoutEnv is env less the named variables.
func withoutEnv(env []string, names ...string) []string {
	out := env[:0:0]
	for _, entry := range env {
		keep := true
		for _, name := range names {
			if len(entry) > len(name) && entry[:len(name)+1] == name+"=" {
				keep = false
			}
		}
		if keep {
			out = append(out, entry)
		}
	}
	return out
}
