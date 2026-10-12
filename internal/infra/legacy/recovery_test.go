package legacy_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A validated name proves no flush: the clearer that made it may have failed
// to flush its rename and refused. A launch that finds one settles the base
// before deleting anything of it — even a launch that bootstrapped, and so
// flushed the base, before that rename was made — and a launch that cannot
// settle refuses and deletes nothing.
func TestAValidatedTreeIsDeletedOnlyOnceSettled(t *testing.T) {
	t.Run("a launch waiting behind a failed flush", func(t *testing.T) {
		s := newScene(t)
		s.letter(s.room(s.rooms(), "old"), "web", "m1")
		ready, resume, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		var mu sync.Mutex
		leases, validating, failed, resumed, settled, unsettled := 0, false, false, false, false, false
		faults(t, func(op, path string) error {
			mu.Lock()
			if op == state.OpStep && path == "leased" {
				if leases++; leases == 1 {
					mu.Unlock()
					close(ready)
					<-resume
					return nil
				}
			}
			defer mu.Unlock()
			switch {
			case op == state.OpRename && strings.Contains(path, "/.rooms-1x-validated-"):
				validating = true
			case op == state.OpSync && path == s.base && validating && !failed:
				failed = true
				return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
			case resumed && op == state.OpSync && path == s.base:
				settled = true
			case resumed && op == state.OpRemove && strings.Contains(path, "/.rooms-1x-validated-") && !settled:
				unsettled = true
			}
			return nil
		})
		go func() { done <- s.launch() }()
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			close(resume)
			t.Fatal("the waiting launch never took its lease")
		}
		err := s.launch()
		mu.Lock()
		resumed = true
		mu.Unlock()
		close(resume)
		var waited error
		select {
		case waited = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("the waiting launch never finished")
		}
		if err == nil || !failed || !strings.Contains(err.Error(), "could not mark") {
			t.Fatalf("the flush of the validated rename did not fail the first launch: %v", err)
		}
		if unsettled {
			t.Fatal("a launch that bootstrapped before the validated rename deleted the tree without settling the base")
		}
		if waited != nil || !settled {
			t.Fatalf("the waiting launch, settled %v: %v", settled, waited)
		}
		s.wantTrees()
	})
	t.Run("a base that cannot be settled", func(t *testing.T) {
		s := newScene(t)
		s.letter(s.room(s.validated(leftID), "prior"), "web", "m-prior")
		mustWrite(t, s.summary(leftID), `{"rooms":1,"unread":1,"undelivered":0}`)
		removed := false
		faults(t, func(op, path string) error {
			switch {
			case op == state.OpSync && path == s.base:
				return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
			case op == state.OpRemove:
				removed = true
			}
			return nil
		})
		s.refused("so no validated tree of rewake 1.x was deleted")
		if removed || s.said.Len() != 0 {
			t.Fatalf("an unsettled tree was deleted (%v) or reported: %q", removed, s.said.String())
		}
		s.wantTrees("validated")
		if !exists(filepath.Join(s.validated(leftID), "prior", "inbox", "web", "m-prior.json")) {
			t.Fatal("the validated tree lost its letter")
		}
		state.Fault = nil
		s.went()
		s.wantTrees()
	})
}

// What an invocation removed or deferred is said on every return once it
// has removed anything, beside the refusal that stopped it later; and each
// refusal names the tree it failed on, never that nothing was removed.
func TestEveryOutcomeIsSaidBesideALaterRefusal(t *testing.T) {
	const removedLine = "rewake: removed the state of rewake 1.x (1 room, 1 unread letter, 0 undelivered reports).\n"
	cases := []struct {
		name, op, path, says string
	}{
		{"setting rooms/ aside", state.OpRename, "/.rooms-1x-candidate-", "could not set %s/rooms aside, so nothing in it was removed"},
		{"recording the counts", state.OpWrite, "/v2/.cleared-1x-", "could not record what %s/.rooms-1x-candidate-"},
		{"marking it validated", state.OpRename, "/.rooms-1x-validated-", "could not mark %s/.rooms-1x-candidate-"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newScene(t)
			s.letter(s.room(s.validated(leftID), "prior"), "web", "m-prior")
			mustWrite(t, s.summary(leftID), `{"rooms":1,"unread":1,"undelivered":0}`)
			s.letter(s.room(s.rooms(), "old"), "web", "m1")
			var mu sync.Mutex
			removedPrior := false
			faults(t, func(op, path string) error {
				mu.Lock()
				defer mu.Unlock()
				if op == state.OpRemove && strings.HasSuffix(path, ".rooms-1x-validated-"+leftID) {
					removedPrior = true
				}
				if removedPrior && op == c.op && strings.Contains(path, c.path) {
					return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
				}
				return nil
			})
			err := s.launch()
			if err == nil || !strings.Contains(err.Error(), strings.ReplaceAll(c.says, "%s", s.base)) || strings.Contains(err.Error(), "nothing was removed") {
				t.Fatalf("the refusal: %v", err)
			}
			if exists(s.validated(leftID)) || s.said.String() != removedLine {
				t.Fatalf("the earlier deletion was not reported beside the refusal: said %q", s.said.String())
			}
		})
	}
	t.Run("a deferred deletion", func(t *testing.T) {
		s := newScene(t)
		s.room(s.validated(leftID), "prior")
		s.letter(s.room(s.rooms(), "old"), "web", "m1")
		faults(t, func(op, path string) error {
			if (op == state.OpRemove || op == state.OpRename) && strings.Contains(path, "/.rooms-1x-") {
				return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
			}
			return nil
		})
		s.refused("could not set " + s.base + "/rooms aside")
		if want := "rewake: 1 validated tree of rewake 1.x could not be deleted yet; the next launch deletes it.\n"; s.said.String() != want {
			t.Fatalf("the deferred deletion was not said beside the refusal: %q", s.said.String())
		}
	})
}
