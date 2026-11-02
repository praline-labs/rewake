package cli

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/host"
	"github.com/praline-labs/rewake/internal/host/worktree"
)

// The image that adopts a run finds the checkout the run's launch made, by
// the process that made it, which the exec keeps; one another process made
// for the same run — worktree_enter's — is not the launch's to settle, and
// sorts first here, so a lookup by the owner alone would take it.
func TestTheAdoptedRunFindsTheCheckoutItsLaunchMade(t *testing.T) {
	lab := newWorktreeLab(t)
	dir := filepath.Join(lab.state, "v2", "rooms", "home")
	owner := worktree.Owner{Name: "writer-claude", Room: "home", Epoch: "41.7.boot", Harness: "claude", Dir: dir}
	entered, err := worktree.Create(lab.root, lab.repo, "entered")
	if err != nil {
		t.Fatal(err)
	}
	entered.Launcher = &worktree.Launch{Epoch: "1.1"}
	if _, err := worktree.Claim(entered, owner); err != nil {
		t.Fatal(err)
	}
	made, err := worktree.Create(lab.root, lab.repo, "made")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Claim(made, owner); err != nil {
		t.Fatal(err)
	}
	harness := harnessProbe(t, agentID)

	found := adoptedCheckout(&host.Handover{Name: owner.Name, Epoch: owner.Epoch, Dir: dir}, harness, io.Discard)
	if found == nil || found.record.Name != "made" || found.launch != "rewake "+harness.ID() {
		t.Fatalf("found %+v, want the checkout named made, launched by rewake %s", found, harness.ID())
	}
	if other := adoptedCheckout(&host.Handover{Name: owner.Name, Epoch: "42.7.boot", Dir: dir}, harness, io.Discard); other != nil {
		t.Fatalf("another run of the name found %s", other.record.Name)
	}
}
