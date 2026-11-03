package pi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// A run handed over is planned on the socket its launch planned, with
// nothing refused or made: arguments a launch refuses, a missing run
// directory's check and pi's directory all belong to a pi not yet started.
func TestAnAdoptionPlansWithoutTheLaunchsSteps(t *testing.T) {
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
	request.Adopting = true
	plan, err := New().Launch(request)
	if err != nil {
		t.Fatalf("the adoption was refused: %v", err)
	}
	if _, ok := plan.Service.(adapter.Handover); !ok || plan.Socket != socket || !plan.OwnsSocket || plan.Wake == nil || plan.TurnBoundary == nil {
		t.Fatalf("plan %+v", plan)
	}
	if _, err := os.Stat(filepath.Join(home, ".config")); !os.IsNotExist(err) {
		t.Fatalf("the adoption made pi's directory: %v", err)
	}
}
