package wrap

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

func TestDefaultRoleStaysGeneralAcrossRoomLifecycle(t *testing.T) {
	for _, h := range harness.All() {
		t.Run(h.ID(), func(t *testing.T) {
			dir := stateDir(t)
			claim := func(prefix string, part role.Role) registry.Session {
				t.Helper()
				session, err := claimName(Request{Dir: dir, Harness: h, Name: prefix, Role: part}, os.Getpid(), selfStart(t), dir)
				if err != nil {
					t.Fatal(err)
				}
				return session
			}
			checkGeneral := func(prefix, wantName string) {
				t.Helper()
				session := claim(prefix, role.Role{})
				if session.Role != role.General.ID || session.Name != wantName || !strings.Contains(session.RoleReason, "default general") || strings.Contains(session.RoleReason, "automatically") {
					t.Fatalf("implicit main or wrong default identity: %+v", session)
				}
			}
			checkGeneral("", "general-"+h.ID())
			claim("writer", role.Write)
			checkGeneral("main", "main-"+h.ID()) // A name is not a role selector.
			checkGeneral("", "general-"+h.ID()+"-2")
			main := claim("leader", role.Main)
			if main.HarnessPID != 0 {
				t.Fatal("fixture unexpectedly started a harness")
			}
			if _, err := claimName(Request{Dir: dir, Harness: h, Name: "second-leader", Role: role.Main}, os.Getpid(), selfStart(t), dir); err == nil {
				t.Fatal("starting main did not reserve the role")
			}
			checkGeneral("", "general-"+h.ID()+"-3")
			if _, err := registry.RemoveOwned(dir, main.Name, main.Epoch()); err != nil {
				t.Fatal(err)
			}
			checkGeneral("", "general-"+h.ID()+"-4")
			replacement := claim("new-leader", role.Main)
			if replacement.Role != role.Main.ID {
				t.Fatal("explicit replacement is not main")
			}
		})
	}
}

func TestDefaultRoleWithOnlyWritersPresent(t *testing.T) {
	dir := stateDir(t)
	if _, err := claimRole(t, dir, "writer", role.Write); err != nil {
		t.Fatal(err)
	}
	session, err := claimRole(t, dir, "", role.Role{})
	if err != nil || session.Role != role.General.ID || session.Name != "general-fake" {
		t.Fatalf("writers triggered election: %+v %v", session, err)
	}
}

func TestConcurrentExplicitMainClaimsHaveOneWinner(t *testing.T) {
	dir := stateDir(t)
	start := selfStart(t)
	gate := make(chan struct{})
	results := make(chan error, 16)
	var group sync.WaitGroup
	for i := range 16 {
		group.Go(func() {
			<-gate
			_, err := claimName(Request{Dir: dir, Harness: &fakeHarness{}, Name: string(rune('a' + i)), Role: role.Main}, os.Getpid(), start, dir)
			results <- err
		})
	}
	close(gate)
	group.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
			continue
		}
		var occupied *MainTakenError
		if !errors.As(err, &occupied) {
			t.Fatalf("wrong concurrent refusal: %v", err)
		}
	}
	if winners != 1 {
		t.Fatalf("explicit main winners=%d", winners)
	}
	sessions, err := registry.List(dir)
	if err != nil || len(sessions) != 1 || sessions[0].Role != role.Main.ID || sessions[0].HarnessPID != 0 {
		t.Fatalf("starting main reservation=%+v %v", sessions, err)
	}
}
