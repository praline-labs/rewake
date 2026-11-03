package claude

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// A worker's run handed over is served on the messaging socket its harness
// listens on: the launch refuses that socket as another session's, which
// ended every Claude Code run a switch adopted (November 3, 2026); Serve
// takes it, serves on the same paths, and writes nothing of the worker
// profile into the service folder.
func TestServePlansOnTheLiveSocket(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	socket := filepath.Join(t.TempDir(), "api.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	request := adapter.LaunchRequest{
		Name: "api", Dir: t.TempDir(), Intro: true, Role: role.Write, Template: "write", Epoch: "e1",
		RunDir: t.TempDir(), Socket: socket, Env: []string{"HOME=" + home},
	}

	if _, err := New().Launch(request); err == nil || !strings.Contains(err.Error(), "is a live socket") {
		t.Fatalf("a launch on the live socket answered %v", err)
	}
	plan := New().Serve(request)
	run, ok := plan.Service.(*modRun)
	switch {
	case !ok:
		t.Fatalf("the adoption's service is %T, which cannot adopt", plan.Service)
	case plan.Socket != socket || !plan.OwnsSocket || run.lane.socket != socket:
		t.Fatalf("socket %q owned %v, lane %q", plan.Socket, plan.OwnsSocket, run.lane.socket)
	case run.socket != modSocketPath(request) || plan.TurnBoundary == nil || plan.Live == nil || plan.Telemetry == nil:
		t.Fatalf("mod socket %q, boundary %v, live %v, telemetry %v", run.socket, plan.TurnBoundary, plan.Live, plan.Telemetry)
	}
	if _, err := os.Stat(socket); err != nil {
		t.Fatalf("the harness's socket went: %v", err)
	}
	if _, err := os.Stat(profileDir(home)); !os.IsNotExist(err) {
		t.Fatalf("the adoption touched the service folder: %v", err)
	}
}
