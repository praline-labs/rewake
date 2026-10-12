package upgraderig

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A launch that finds the clear lock held — another launch paused in its
// clearing, a real process — waits the clear bound and no longer, then
// refuses, changing nothing; once the first is done, it goes on.
func TestClearLockTimeoutRefuses(t *testing.T) {
	w := newWorld(t)
	w.standin("letter", filepath.Join(w.base, "rooms"), "old", "web", "m1")
	first := w.launch("r1", "other:holdat=legacy admitted,launch cleared@PAUSE")
	if step := first.heldAt(); step != "legacy admitted" {
		t.Fatalf("the first launch held at %q: %s", step, first.stderr.String())
	}
	started := time.Now()
	end, said := w.launch("r2", "other:holdat=launch cleared@PAUSE").end()
	waited := time.Since(started)
	if end != refused || !strings.Contains(said, "Rewake: another launch is checking the state of rewake 1.x; launch again.") {
		t.Fatalf("the second launch: %s %q", end, said)
	}
	if waited < clearWait || waited > clearWait+5*time.Second {
		t.Fatalf("the second launch refused after %s, its bound %s", waited, clearWait)
	}
	if got := w.trees(); strings.Join(got, ",") != "rooms" {
		t.Fatalf("a refused launch changed the trees: %v", got)
	}
	first.resume()
	if end, said := first.end(); end != went || !strings.Contains(said, "removed the state of rewake 1.x (1 room, 1 unread letter, 0 undelivered reports)") {
		t.Fatalf("the first launch: %s %q", end, said)
	}
	if end, said := w.launch("r2", "other:holdat=launch cleared@PAUSE").end(); end != went || said != "" {
		t.Fatalf("the second launch again: %s %q", end, said)
	}
}

// A launch that read the trees before the clear lock acts on what it finds
// once it holds the lock: the first read only chose the slow path. Behind a
// clearer that removes rooms/ while it waits, it goes on and says nothing.
// The second launch is held at "legacy found", after its first read and
// before it asks for the clear lock, until the first is let go: an order the
// test sets rather than a time it waits.
func TestTheClearingActsOnWhatItFindsUnderTheLock(t *testing.T) {
	w := newWorld(t)
	w.standin("letter", filepath.Join(w.base, "rooms"), "old", "web", "m1")
	first := w.launch("r1", "other:holdat=legacy admitted,launch cleared@PAUSE")
	if step := first.heldAt(); step != "legacy admitted" {
		t.Fatalf("the first launch held at %q: %s", step, first.stderr.String())
	}
	second := w.launch("r2", "other:holdat=legacy found,launch cleared@PAUSE")
	if step := second.heldAt(); step != "legacy found" {
		t.Fatalf("the second launch held at %q: %s", step, second.stderr.String())
	}
	second.resume()
	first.resume()
	if end, said := first.end(); end != went || !strings.Contains(said, "removed the state of rewake 1.x") {
		t.Fatalf("the first launch: %s %q", end, said)
	}
	if end, said := second.end(); end != went || said != "" {
		t.Fatalf("the second launch, behind the first: %s %q", end, said)
	}
}
