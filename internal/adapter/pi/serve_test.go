package pi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// A run handed over is served on the socket its launch planned, with nothing
// refused or made: arguments a launch refuses and pi's directory belong to a
// pi not yet started.
func TestServePlansWithoutTheLaunchsSteps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	socket := filepath.Join(t.TempDir(), "api.sock")
	request := adapter.LaunchRequest{
		Name: "api", Role: role.Write, Epoch: "e1", Intro: true,
		RunDir: t.TempDir(), Socket: socket, Args: []string{sessionDirFlag, "/elsewhere"},
	}
	if _, err := New().Launch(request); err == nil {
		t.Fatal("a launch took a worker's own session directory")
	}
	plan := New().Serve(request)
	if _, ok := plan.Service.(adapter.Handover); !ok || plan.Socket != socket || !plan.OwnsSocket || plan.Wake == nil || plan.TurnBoundary == nil {
		t.Fatalf("plan %+v", plan)
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
		t.Fatalf("the adoption made pi's directory: %v", err)
	}
}
