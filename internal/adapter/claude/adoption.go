package claude

import (
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
	"github.com/praline-labs/rewake/internal/core/role"
)

// What a launch and an adoption of a run share: the arguments as far as the
// run's service reads them, its reply socket and its service. Everything
// else of Launch — the stale socket's removal, the worker profile written
// into the service folder, the briefing, the refusals of the arguments —
// prepares a harness that is not running yet, and an adoption repeats none
// of it (adapter.LaunchRequest.Adopting, docs/claude.md#handing-the-run-over).

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

// adoptionPlan is the plan of a run handed over: the socket and the service
// its launch planned, from the same request, with nothing checked or
// written. The harness listens on the messaging socket, so the stale
// socket's check would call it another session's (docs/traps.md, November
// 3, 2026). The defaults are read again from rewake's settings: a default
// changed since the launch moves only the effort the endpoint reports until
// the status line says the run's own.
func adoptionPlan(request adapter.LaunchRequest) adapter.LaunchPlan {
	args := request.Args
	if defaulted, _, err := withDefaults(request, args); err == nil {
		args = defaulted
	}
	socket, owns := request.Socket, true
	if adapter.HasFlag(args, socketFlag) {
		socket, owns = flagValue(args, socketFlag), false
	}
	reply, _ := replyOf(request, socket, owns)
	wake := newLane(socket, reply, owns)
	wake.room = request.RoomDir
	run := serve(request, args, wake)
	return adapter.LaunchPlan{
		Socket: socket, OwnsSocket: owns,
		Service: run.service, Wake: wake, Live: run.live, TurnBoundary: run.turns, Telemetry: run.telemetry,
	}
}
