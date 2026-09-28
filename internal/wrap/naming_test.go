package wrap

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	_ "github.com/praline-labs/rewake/internal/harness/catalog"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

type namedHarness struct {
	fakeHarness
	id string
}

func (h *namedHarness) ID() string { return h.id }

func TestLaunchNamesFollowSelectedRoleAndHarness(t *testing.T) {
	for _, h := range harness.All() {
		for _, part := range role.All() {
			t.Run(h.ID()+"/"+part.ID, func(t *testing.T) {
				dir := stateDir(t)
				session, err := claimName(Request{Dir: dir, Harness: h, Role: part}, os.Getpid(), selfStart(t), dir)
				if err != nil || session.Name != part.ID+"-"+h.ID() || session.Role != part.ID {
					t.Fatalf("claim=%+v err=%v", session, err)
				}
			})
		}
	}
	dir := stateDir(t)
	for _, want := range []string{"general-fake", "general-fake-2", "general-fake-3", "general-fake-4"} {
		session, err := claimRole(t, dir, "", role.Role{})
		if err != nil || session.Name != want {
			t.Fatalf("claim=%+v err=%v want=%s", session, err, want)
		}
	}
}

func TestExplicitPrefixControlsNameButNotRole(t *testing.T) {
	dir := stateDir(t)
	for _, prefix := range []string{"megamozg", "already-fake"} {
		session, err := claimRole(t, dir, prefix, role.Write)
		if err != nil || session.Name != prefix+"-fake" || session.Role != role.Write.ID {
			t.Fatalf("claim=%+v err=%v", session, err)
		}
	}
	_, err := claimRole(t, dir, "megamozg", role.General)
	var taken *registry.NameTakenError
	if !errors.As(err, &taken) || taken.Name != "megamozg-fake" {
		t.Fatalf("explicit conflict=%v", err)
	}
	other, err := state.RoomDir(state.RootForRoom(dir), "other")
	if err != nil {
		t.Fatal(err)
	}
	if session, err := claimRole(t, other, "megamozg", role.Write); err != nil || session.Name != "megamozg-fake" {
		t.Fatalf("other room=%+v err=%v", session, err)
	}
}

func TestNamePrefixAndAssembledBoundaries(t *testing.T) {
	for _, prefix := range []string{"Upper", "has space", "../escape", "-leading", "é", strings.Repeat("p", 33), strings.Repeat("p", 28), strings.Repeat("p", 32)} {
		t.Run(prefix, func(t *testing.T) {
			dir := stateDir(t)
			_, err := claimRole(t, dir, prefix, role.Write)
			if !errors.Is(err, registry.ErrUnusableName) || !strings.Contains(err.Error(), "prefix") {
				t.Fatalf("invalid prefix accepted or no hint: %v", err)
			}
			if sessions, _ := registry.List(dir); len(sessions) != 0 {
				t.Fatal("invalid name published a session")
			}
		})
	}
	dir := stateDir(t)
	for _, prefix := range []string{"p", strings.Repeat("p", 27), "a.b_c-1"} {
		session, err := claimRole(t, dir, prefix, role.Write)
		if err != nil || session.Name != prefix+"-fake" {
			t.Fatalf("valid boundary=%+v err=%v", session, err)
		}
	}
}

func TestGenericHarnessAndAutomaticSuffixLength(t *testing.T) {
	dir := stateDir(t)
	h := &namedHarness{id: "engine.v2"}
	session, err := claimName(Request{Dir: dir, Harness: h, Role: role.Write}, os.Getpid(), selfStart(t), dir)
	if err != nil || session.Name != "write-engine.v2" {
		t.Fatalf("generic claim=%+v err=%v", session, err)
	}
	for _, length := range []int{24, 25, 26} {
		t.Run(string(rune('a'+length)), func(t *testing.T) {
			dir := stateDir(t)
			h := &namedHarness{id: strings.Repeat("x", length)}
			request := Request{Dir: dir, Harness: h, Role: role.Write}
			first, err := claimName(request, os.Getpid(), selfStart(t), dir)
			if err != nil {
				t.Fatal(err)
			}
			second, err := claimName(request, os.Getpid(), selfStart(t), dir)
			if length == 24 {
				if err != nil || second.Name != first.Name+"-2" || len(second.Name) != 32 {
					t.Fatalf("suffix boundary=%+v err=%v", second, err)
				}
			} else if !errors.Is(err, registry.ErrUnusableName) || !strings.Contains(err.Error(), "prefix") {
				t.Fatalf("oversized automatic suffix=%+v err=%v", second, err)
			}
		})
	}
}

func TestConcurrentAutomaticNamesUseDefaultGeneral(t *testing.T) {
	dir := stateDir(t)
	start := selfStart(t)
	results := make(chan registry.Session, 8)
	failures := make(chan error, 8)
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			session, err := claimName(Request{Dir: dir, Harness: &fakeHarness{}}, os.Getpid(), start, dir)
			if err != nil {
				failures <- err
			} else {
				results <- session
			}
		})
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	names := map[string]bool{}
	for session := range results {
		if names[session.Name] {
			t.Fatalf("duplicate address %s", session.Name)
		}
		names[session.Name] = true
		if session.Role != role.General.ID || !strings.HasPrefix(session.Name, "general-fake") {
			t.Fatal(session)
		}
	}
	if len(names) != 8 {
		t.Fatalf("names=%v", names)
	}
}

func TestHarnessLengthLeavesRoomForAPrefix(t *testing.T) {
	dir := stateDir(t)
	for _, size := range []int{30, 31, 32} {
		h := &namedHarness{id: strings.Repeat("h", size)}
		session, err := claimName(Request{Dir: dir, Name: "p", Harness: h, Role: role.Write}, os.Getpid(), selfStart(t), dir)
		if size == 30 {
			if err != nil || len(session.Name) != 32 {
				t.Fatalf("maximum harness suffix: %+v %v", session, err)
			}
		} else if !errors.Is(err, registry.ErrUnusableName) || !strings.Contains(err.Error(), "shorter ID") {
			t.Fatalf("impossible prefix budget has no actionable error: %v", err)
		}
	}
}
