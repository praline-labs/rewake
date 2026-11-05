package delivery

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func freshStallSnapshot(activity string) session.Snapshot {
	return session.Snapshot{Activity: &activity, ActivityFresh: true, Fresh: true}
}

// The existing lock-wait seam places the change after preflight but before
// rejudgment, without sleeping or publishing a turn while holding its lock.
func beforeStallLock(t *testing.T, s *Server, change func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	s.lockContext = ctx
	held, release := make(chan struct{}), make(chan struct{})
	unlock := sync.OnceFunc(func() { close(release) })
	defer unlock()
	released := make(chan error, 1)
	go func() {
		released <- s.box().WithLock(ctx, func(mail.Locked) error { close(held); <-release; return nil })
	}()
	select {
	case <-held:
	case <-ctx.Done():
		t.Fatal("the other holder never took the lock")
	}
	before := state.LockWait
	defer func() { state.LockWait = before }()
	changed := false
	state.LockWait = func(path string) {
		if path != state.InboxPath(s.Dir, s.Name) {
			return
		}
		state.LockWait = before
		unlock()
		if err := <-released; err != nil {
			t.Fatal(err)
		}
		change()
		changed = true
	}
	s.remindStalled(ctx)
	if !changed {
		t.Fatal("the preflight never reached the mailbox lock")
	}
}

// Neither harness needs to record a timed start for live working activity to
// suppress the letter. Sampling only before the lock would wake this worker.
func TestASecondLiveSampleCanEndAStallWithoutATimedStart(t *testing.T) {
	var notices []mail.Message
	s := stalledServer(t, &notices)
	looks := 0
	s.StallSnapshot = func() session.Snapshot {
		looks++
		activity := session.ActivityIdle
		if looks > 1 {
			activity = session.ActivityWorking
		}
		return freshStallSnapshot(activity)
	}
	s.remindStalled(context.Background())
	if looks != 2 || len(stallLetters(t, s)) != 0 || s.reminded != "" {
		t.Fatalf("samples %d, letters %+v, remembered %q", looks, stallLetters(t, s), s.reminded)
	}
}

// A new interim while waiting is judged on its own bound, text and identity,
// never on the op the pass found before it took the lock.
func TestANewInterimDecidesTheReminder(t *testing.T) {
	for _, due := range []bool{false, true} {
		t.Run(map[bool]string{false: "not due", true: "due"}[due], func(t *testing.T) {
			var notices []mail.Message
			s := stalledServer(t, &notices)
			s.StallSnapshot = func() session.Snapshot { return freshStallSnapshot(session.ActivityIdle) }
			old, _, _ := s.box().Stalled()
			beforeStallLock(t, s, func() {
				ended := boottime.Now()
				if due {
					ended -= int64(1500 * time.Millisecond)
				}
				if err := mail.MarkPending(s.Dir, s.Name, s.Epoch, mail.MarkName(ended-1, mail.NewID()), "another run is pending", ended-1); err != nil {
					t.Fatal(err)
				}
				self := must(registry.Lookup(s.Dir, s.Name))
				if outcome, err := mail.CompleteTurn(t.Context(), s.Dir, self, mail.TurnEnd{Text: "still going", Started: old.Ended + 1, Ended: ended}, "t", mail.TurnEndOptions{}); err != nil || !outcome.Interim {
					t.Fatalf("new interim: %+v %v", outcome, err)
				}
			})
			letters := stallLetters(t, s)
			if !due {
				if len(letters) != 0 || s.reminded != "" {
					t.Fatalf("new interim not yet due: %+v, %s", letters, s.reminded)
				}
				return
			}
			next, _, _ := s.box().Stalled()
			if len(letters) != 1 || !strings.Contains(letters[0].Text, `"another run is pending"`) || s.reminded != next.Op || s.reminded == old.Op {
				t.Fatalf("new interim %+v, letters %+v, remembered %s", next, letters, s.reminded)
			}
		})
	}
}
