package catalog

import (
	"fmt"
	"io/fs"
	"maps"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// Every adapter's Serve is pure: it is all a run handed over to the next
// image of its wrapper is planned with, while its harness runs on, and a step
// of a launch reached from it fails on the running harness or changes what it
// runs on. The adoption of every Claude Code run met its launch's refusal of
// the live messaging socket and ended the session (November 3, 2026). So,
// for main and a worker: no program runs — no version read, no probe —,
// nothing is written under the home, the room, the run's directory or the
// socket's, the live socket is left to its harness, and the plan serves on it.
// A launch-only step that lands in Serve fails here by what it does; one that
// only refuses cannot, since Serve answers no error. The request names no
// --command: a launch through one skips steps an ordinary launch takes.
func TestEveryAdaptersServeIsPure(t *testing.T) {
	for _, h := range New().All() {
		for _, part := range []string{"main", "write"} {
			t.Run(h.ID()+"/"+part, func(t *testing.T) {
				run := newPureRun(t, part)
				plan := h.Serve(run.request)
				run.check(t, plan)
			})
		}
	}
}

// Launch composes Serve: the run it starts is served on the same socket, with
// the same capabilities, as the run handed over will be.
func TestEveryAdaptersLaunchComposesItsServe(t *testing.T) {
	for _, h := range New().All() {
		t.Run(h.ID(), func(t *testing.T) {
			run := newPureRun(t, "write")
			run.request.Socket = filepath.Join(filepath.Dir(run.request.Socket), "fresh.sock")
			launched, err := h.Launch(run.request)
			if err != nil {
				t.Fatalf("the launch was refused: %v", err)
			}
			served := h.Serve(run.request)
			if got, want := shape(launched.ServicePlan), shape(served); got != want {
				t.Fatalf("the launch serves %s, Serve %s", got, want)
			}
		})
	}
}

// pureRun is a request whose every path is the test's, beside a trap for any
// program run and a harness listening on the messaging socket.
type pureRun struct {
	request  adapter.LaunchRequest
	marks    string
	watched  []string
	before   map[string]string
	listener net.Listener
}

// newPureRun names the run's role by its id alone, which is what an adapter
// tells main from a worker by: the layout keeps the core out of the catalog.
func newPureRun(t *testing.T, part string) *pureRun {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, name := range []string{"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(name, filepath.Join(home, name))
	}
	// A short directory: a unix socket's path has a limit.
	sockets, err := os.MkdirTemp("", "rw")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockets) })
	socket := filepath.Join(sockets, "api.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	traps, marks := t.TempDir(), t.TempDir()
	for _, program := range []string{"claude", "pi", "node", "bun", "fixture", "git", "trap"} {
		trap := fmt.Sprintf("#!/bin/sh\necho \"$*\" > %q\n", filepath.Join(marks, program))
		if err := os.WriteFile(filepath.Join(traps, program), []byte(trap), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", traps)

	room, runDir, control := t.TempDir(), t.TempDir(), t.TempDir()
	run := &pureRun{
		request: adapter.LaunchRequest{
			Name: "api", Dir: filepath.Dir(room), RoomDir: room, Room: "room", RoleReason: "the test's",
			Intro: true, Socket: socket, Epoch: "1.1.boot", Template: "write",
			Executable: filepath.Join(traps, "trap"),
			ControlDir: control, RunDir: runDir, Version: "9.9.9",
			Start: adapter.Start{Command: filepath.Join(traps, "trap")},
			Env:   []string{"HOME=" + home, "PATH=" + traps},
		},
		marks:    marks,
		watched:  []string{home, room, runDir, control, sockets},
		listener: listener,
	}
	run.request.Role.ID = part
	run.before = run.tree(t)
	return run
}

// tree is every path under the watched directories, with its kind, size and
// time of change.
func (r *pureRun) tree(t *testing.T) map[string]string {
	t.Helper()
	found := map[string]string{}
	for _, root := range r.watched {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			found[path] = fmt.Sprintf("%v %d %d", info.Mode(), info.Size(), info.ModTime().UnixNano())
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return found
}

func (r *pureRun) check(t *testing.T, plan adapter.ServicePlan) {
	t.Helper()
	if ran, _ := os.ReadDir(r.marks); len(ran) > 0 {
		t.Errorf("Serve ran a program: %s", ran[0].Name())
	}
	if after := r.tree(t); !maps.Equal(after, r.before) {
		t.Errorf("Serve changed the files: before %v, after %v", r.before, after)
	}
	conn, err := net.Dial("unix", r.request.Socket)
	if err != nil {
		t.Fatalf("the harness's socket no longer answers: %v", err)
	}
	_ = conn.Close()
	if plan.Socket != r.request.Socket || !plan.OwnsSocket || plan.Service == nil {
		t.Fatalf("Serve planned socket %q owned %v, service %T", plan.Socket, plan.OwnsSocket, plan.Service)
	}
}

// shape is what a service plan serves: its socket and which capabilities it
// carries.
func shape(plan adapter.ServicePlan) string {
	return fmt.Sprintf("socket %s owned %v service %T wake %v boundary %v telemetry %v thread %v live %v",
		plan.Socket, plan.OwnsSocket, plan.Service, plan.Wake != nil, plan.TurnBoundary != nil,
		plan.Telemetry != nil, plan.Thread != nil, plan.Live != nil)
}
