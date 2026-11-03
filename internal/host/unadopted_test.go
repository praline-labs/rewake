package host

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// refusingAdoption plans a service that cannot adopt what it is handed, as
// the adoption of every Claude Code run failed on November 3, 2026.
type refusingAdoption struct{ *fakeHarness }

// unadoptable is a service with no Adopt.
type unadoptable struct{}

func (unadoptable) Start(context.Context, func(string)) error { return nil }
func (unadoptable) Close()                                    {}

func (r refusingAdoption) Serve(adapter.LaunchRequest) adapter.ServicePlan {
	return adapter.ServicePlan{Service: unadoptable{}, Wake: r.fakeHarness}
}

// A worker's adoption that fails before it has its harness holds the
// harness rather than exit under it: the harness runs on, the run's record
// stays and main is told, and the run's record and directory go once the
// harness has ended, with its exit code.
func TestAFailedAdoptionHoldsTheHarness(t *testing.T) {
	dir := stateDir(t)
	harness := refusingAdoption{&fakeHarness{}}
	self := os.Getpid()
	selfStart, err := proc.StartTime(self)
	if err != nil {
		t.Fatal(err)
	}
	boot := mustBoot(t)
	claim := func(name string, part role.Role) registry.Session {
		session, err := claimName(Request{Harness: harness, Dir: dir, Name: name, Role: part}, self, selfStart, boot, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	lead := claim("lead", role.Main)
	worker := claim("api", role.Write)
	runDir, err := makeRunDir(dir, worker.Name, worker.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill() })
	childStart, err := proc.StartTime(child.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	handover := &Handover{
		Version: HandoverVersion, Harness: harness.ID(), Dir: dir, Name: worker.Name, Epoch: worker.Epoch(),
		Role: role.Write.ID, HarnessPID: child.Process.Pid, HarnessStart: childStart,
	}
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() {
		code, err := Run(context.Background(), handover.Request(harness))
		done <- result{code, err}
	}()

	told := func() bool {
		entries, _ := os.ReadDir(state.InboxPath(dir, lead.Name))
		for _, entry := range entries {
			raw, _ := os.ReadFile(filepath.Join(state.InboxPath(dir, lead.Name), entry.Name()))
			if strings.Contains(string(raw), "could not be adopted: the run was handed over to an adapter that cannot adopt it") {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(5 * time.Second)
	for !told() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !told() {
		t.Fatal("main was not told of the failed adoption")
	}
	time.Sleep(200 * time.Millisecond)
	select {
	case got := <-done:
		t.Fatalf("the run returned under its harness: %d, %v", got.code, got.err)
	default:
	}
	if !proc.Alive(child.Process.Pid, childStart) {
		t.Fatal("the harness did not run on")
	}
	if _, err := registry.Load(dir, worker.Name); err != nil {
		t.Fatalf("the held run's record went: %v", err)
	}
	if mark, _ := os.ReadFile(filepath.Join(runDir, reexecMark)); !strings.HasPrefix(string(mark), "not adopted: ") {
		t.Fatalf("the sender of the exec reads %q", mark)
	}

	_ = child.Process.Kill()
	var got result
	select {
	case got = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the run did not end with its harness")
	}
	if got.code != 137 || got.err == nil {
		t.Fatalf("the run ended %d, %v; want the harness's 137 and the adoption's error", got.code, got.err)
	}
	if _, err := registry.Load(dir, worker.Name); err == nil {
		t.Fatal("the record outlived the harness")
	}
	if _, err := os.Lstat(runDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the run's directory outlived the harness: %v", err)
	}
}
