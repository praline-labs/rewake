package spacehost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/worktree"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

// A worker whose session claimed its checkout before the space host first
// looked, with main ending first: the space is already closing when the
// checkout is found, and the close still records it — kept while it is there,
// never closed as a space that made none.
func TestACheckoutClaimedBeforeAnyPollIsKeptWhenMainEndsFirst(t *testing.T) {
	dir := buildtest.Dir(t)
	root := t.TempDir()
	t.Setenv(worktree.RootEnv, root)
	const workerRun = "4242.1750.boot"

	checkout := filepath.Join(root, "repo-1a2b", "fix-login")
	if err := os.MkdirAll(checkout, 0o700); err != nil {
		t.Fatal(err)
	}
	claim, err := json.Marshal(worktree.Record{
		Name: "fix-login", Repository: "repo-1a2b", Branch: "fix-login", Path: checkout,
		CreatedAt: time.Now().UTC(),
		Session:   &worktree.Owner{Name: "api-claude", Epoch: workerRun, Dir: dir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "repo-1a2b", "fix-login.json"), claim, 0o600); err != nil {
		t.Fatal(err)
	}

	id, err := space.NewID()
	if err != nil {
		t.Fatal(err)
	}
	if err := space.Create(dir, space.Record{
		ID: id, Name: "api",
		Main:     space.Run{Name: "main-claude", Run: "100.1700.boot"},
		Worker:   space.Worker{Harness: "claude", Name: "api", Role: "write"},
		OpenedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := space.SetHost(dir, id, "200.1720.boot"); err != nil {
		t.Fatal(err)
	}
	if _, err := space.Opened(dir, id, workerRun); err != nil {
		t.Fatal(err)
	}
	record, err := space.BeginClosing(dir, id, "main's run ended")
	if err != nil {
		t.Fatal(err)
	}

	h := &host{spec: Spec{Dir: dir, ID: id, Worktree: true}, record: record}
	if err := h.close("main's run ended"); err != nil {
		t.Fatal(err)
	}
	closed, err := space.Load(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	if closed.State != space.Kept || closed.Checkout.Path != checkout || closed.Checkout.Branch != "fix-login" || closed.End.Checkout.Presence != space.There {
		t.Fatalf("the space closed as %s with %+v, found %+v", closed.State, closed.Checkout, closed.End.Checkout)
	}
}
