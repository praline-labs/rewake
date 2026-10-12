package legacy_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/legacy"
)

// The clear lock waits 10 s and each room's launch lock 2 s, with no total
// beside: a launch with more rooms than fit the clear bound says it is
// checking them, and how long each may take; a room whose launch lock a 1.x
// command holds is waited for its bound and no longer.
func TestTheClearingWaitsKeepTheirBounds(t *testing.T) {
	if clearing, room := legacy.Waits(); clearing != 10*time.Second || room != 2*time.Second {
		t.Fatalf("the bounds as built: clear %s, room %s", clearing, room)
	}
	s := newScene(t)
	for _, room := range []string{"a", "b", "c", "d"} {
		s.room(s.rooms(), room)
	}
	s.went()
	if strings.Contains(s.said.String(), "checking") {
		t.Fatalf("four rooms within the bound were announced: %q", s.said.String())
	}
	for _, room := range []string{"a", "b", "c", "d", "e"} {
		s.room(s.rooms(), room)
	}
	s.said.Reset()
	s.went()
	if !strings.HasPrefix(s.said.String(), "rewake: checking 5 rooms of rewake 1.x, up to 50ms each\n") {
		t.Fatalf("five rooms past the bound: %q", s.said.String())
	}

	busy := newScene(t)
	holdLock(t, filepath.Join(busy.room(busy.rooms(), "old"), ".launch.lock"))
	started := time.Now()
	busy.refused("a rewake 1.x command runs in room old")
	if waited := time.Since(started); waited < 50*time.Millisecond || waited > 500*time.Millisecond {
		t.Fatalf("a held launch lock was waited for %s, its bound 50ms", waited)
	}
}
