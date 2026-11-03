package claude

import (
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
	"github.com/praline-labs/rewake/internal/core/role"
)

// The run's service, which Launch composes and an adoption plans alone
// (adapter.Harness.Serve): the arguments as far as the service reads them,
// the reply socket and the service. Everything else of Launch — the stale
// socket's removal, the worker profile written into the service folder, the
// briefing, the refusals of the arguments — prepares a harness that is not
// running yet, and is not reached from here (docs/claude.md#what-an-adoption-repeats).

// withDefaults is the caller's arguments with rewake's defaults and a
// worker's window.
func withDefaults(request adapter.LaunchRequest, args []string) ([]string, []string, error) {
	args, notes, err := adapter.ApplyDefaults(args, claudeDefaults())
	if err != nil {
		return nil, nil, err
	}
	if request.Role.ID != role.Main.ID && !adapter.HasFlag(args, autocompactFlag) {
		args = adapter.AddFlags(args, autocompactFlag, defaultWindow)
	}
	return args, notes, nil
}

// replyOf is the run's reply socket, "" with a note when its path is too
// long for one.
func replyOf(request adapter.LaunchRequest, socket string, owns bool) (string, string) {
	reply := replyPath(request, socket, owns)
	if len(reply) > maxSocketPath {
		return "", "not hearing back about held messages: the socket path " + reply + " is longer than a unix socket allows"
	}
	return reply, ""
}

// served is the run's service and what it reports through; modSocket is the
// mod's socket when the run loads the mod, "" when it does not.
type served struct {
	service   adapter.Service
	live      adapter.LiveReporter
	turns     adapter.TurnBoundary
	telemetry adapter.Telemetry
	modSocket string
}

// serve builds the run's service on its lane: the mod's endpoint beside it,
// or the lane alone and why there is no mod.
func serve(request adapter.LaunchRequest, args []string, wake *lane) served {
	modSocket := modSocketPath(request)
	switch {
	case len(modSocket) > maxSocketPath:
		return served{service: modless{lane: wake, cause: "its socket path " + modSocket + " is longer than a unix socket allows; start it with a shorter session name, or point REWAKE_DIR, the state directory, at a shorter path"}}
	case request.RunDir == "" || request.Socket == "":
		return served{service: modless{lane: wake, cause: "the launch has no run directory or inbox socket of rewake's to load it from"}}
	}
	run := &modRun{lane: wake, dir: modDir(request.RunDir), socket: modSocket, hostsSpaces: request.Role.Spaces, controls: request.ControlDir != ""}
	run.server = modhost.New(modhost.Config{
		Socket: modSocket, Role: request.Role.ID, Record: filepath.Join(request.RunDir, "hello.json"), Wait: harnessWait,
		ControlDir: request.ControlDir, Epoch: request.Epoch,
		Live: run.transition, Ended: runEnded,
		Asked: askedOf(request), Dialog: dialogOf(request), Effort: flagValue(args, effortFlag), Window: windowTokens(flagValue(args, autocompactFlag)), Closing: request.Closing, Mirror: request.Mirror, Work: request.Work,
		RoomDir: request.RoomDir,
	})
	return served{service: run, live: run, turns: run, telemetry: run.server, modSocket: modSocket}
}

// runService is the run's service plan, with what the launch adds for it:
// the mod's socket when the run loads the mod, and a note when the run hears
// nothing back about held messages.
type runService struct {
	plan            adapter.ServicePlan
	modSocket, note string
}

// serviceOf plans the run's service from the request and args, the
// arguments the harness runs with as far as the service reads them: the
// effort and the window it reports. The messaging socket is the caller's
// when the caller's own arguments name it, and rewake's on request.Socket
// otherwise. Pure: it reads and writes no file.
func serviceOf(request adapter.LaunchRequest, args []string) runService {
	socket, owns := request.Socket, true
	if adapter.HasFlag(request.Args, socketFlag) {
		socket, owns = flagValue(request.Args, socketFlag), false
	}
	reply, note := replyOf(request, socket, owns)
	wake := newLane(socket, reply, owns)
	wake.room = request.RoomDir
	run := serve(request, args, wake)
	return runService{
		plan: adapter.ServicePlan{
			Socket: socket, OwnsSocket: owns,
			Service: run.service, Wake: wake, Live: run.live, TurnBoundary: run.turns, Telemetry: run.telemetry,
		},
		modSocket: run.modSocket, note: note,
	}
}

// Serve is the service of a run handed over. Its harness listens on the
// messaging socket, so the launch's check of a stale socket would call it
// another session's (docs/traps.md, November 3, 2026). The defaults are read
// again from rewake's settings, and one that cannot be read leaves the
// caller's arguments: a default changed since the launch moves only the
// effort the endpoint reports until the status line says the run's own.
func (claudeHarness) Serve(request adapter.LaunchRequest) adapter.ServicePlan {
	args := request.Args
	if defaulted, _, err := withDefaults(request, args); err == nil {
		args = defaulted
	}
	return serviceOf(request, args).plan
}
