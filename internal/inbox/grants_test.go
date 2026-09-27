package inbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

func grantPending(t *testing.T, dir string, grants ...bool) []Message {
	t.Helper()
	var result []Message
	start := time.Now().Add(-time.Minute)
	for index, granted := range grants {
		m := message(fmt.Sprintf("task %d", index))
		m.Kind, m.FromEpoch, m.ToEpoch = Task, "sender-epoch", "receiver-epoch"
		m.CreatedAt = start.Add(time.Duration(index) * time.Second)
		if granted {
			m.GrantDirs = []string{"/work/lib"}
		}
		if err := Put(dir, m); err != nil {
			t.Fatal(err)
		}
		result = append(result, m)
	}
	return result
}

func ids(messages []Message) []string {
	var result []string
	for _, m := range messages {
		result = append(result, m.ID)
	}
	return result
}

// A task carrying a grant goes on its own notice, and while it waits for the
// reader to be idle the mail around it goes on without it.
func TestGrantedTaskGoesAloneAndItsWaitHoldsNothingBack(t *testing.T) {
	dir := stateDir(t)
	members := grantPending(t, dir, false, true, false)
	s := batchServer(dir)
	s.CheckGrant = func(Message) error { return nil }
	idle := false
	var notices [][]string
	s.Reserve = func(_ context.Context, m Message) (Reservation, error) {
		if CarriesGrant(m) && !idle {
			return nil, fmt.Errorf("%w: waiting for idle", ErrNotYet)
		}
		r := &reservationFixture{}
		r.prepare = func(fn func(string) error) error { return fn("root-A") }
		r.deliver = func(_ context.Context, m Message) Result {
			batch := m.Batch
			if len(batch) == 0 {
				batch = []Message{m}
			}
			notices = append(notices, ids(batch))
			return Result{State: Delivered}
		}
		return r, nil
	}
	for range 3 {
		s.drain(context.Background())
	}
	// The task after the granted one joins the notice before it: the grant
	// waits, and nothing waits for the grant.
	want := [][]string{{members[0].ID, members[2].ID}}
	if !slices.EqualFunc(notices, want, slices.Equal) {
		t.Fatalf("notices = %v, want %v", notices, want)
	}
	if available, _ := AvailableUnread(dir, "api", s.Epoch); slices.Contains(ids(available), members[1].ID) {
		t.Fatal("the granted task became readable while its reader was busy")
	}
	idle, s.attempts = true, map[string]time.Time{}
	s.drain(context.Background())
	if len(notices) != 2 || !slices.Equal(notices[1], []string{members[1].ID}) {
		t.Fatalf("notices = %v", notices)
	}
}

// A grant that does not pass at delivery — a directory changed, or main's
// wrapper did not confirm it — fails its task before anything is reserved, and
// its sender is told why.
func TestGrantThatDoesNotPassIsNotDelivered(t *testing.T) {
	for name, check := range map[string]func(Message) error{
		"changed":       func(Message) error { return errors.New("/work/lib now leads to /etc: a link changed") },
		"not confirmed": func(Message) error { return errors.New("main did not confirm the grant") },
		"no check":      nil,
	} {
		t.Run(name, func(t *testing.T) {
			dir := stateDir(t)
			members := grantPending(t, dir, true)
			s := batchServer(dir)
			s.CheckGrant = check
			s.Reserve = func(context.Context, Message) (Reservation, error) {
				t.Fatal("a failed grant was reserved")
				return nil, nil
			}
			s.drain(context.Background())
			status, ok := ReadStatus(dir, "api", members[0].ID)
			if !ok || status.State != Failed || !strings.Contains(status.Detail, "does not pass") {
				t.Fatalf("status = %+v", status)
			}
			told, err := list(dir, "web")
			if err != nil || len(told) != 1 || told[0].Undelivered == nil || told[0].Undelivered.ID != members[0].ID {
				t.Fatalf("sender was not told: %+v %v", told, err)
			}
		})
	}
}

// A sender's wrapper that is there but did not answer yet leaves the task
// waiting, and a later pass that hears from it delivers the task.
func TestGrantNotConfirmedYetWaits(t *testing.T) {
	dir := stateDir(t)
	members := grantPending(t, dir, true)
	s := batchServer(dir)
	s.Deliver = func(context.Context, Message) Result { return Result{State: Delivered} }
	answered := false
	s.CheckGrant = func(Message) error {
		if !answered {
			return fmt.Errorf("%w: the wrapper of web did not confirm the grant yet", ErrNotYet)
		}
		return nil
	}
	s.drain(context.Background())
	if status, ok := ReadStatus(dir, "api", members[0].ID); !ok || status.State != Pending || !strings.Contains(status.Detail, "did not confirm the grant yet") {
		t.Fatalf("status = %+v", status)
	}
	answered, s.attempts = true, map[string]time.Time{}
	s.drain(context.Background())
	if status, ok := ReadStatus(dir, "api", members[0].ID); !ok || status.State != Delivered {
		t.Fatalf("status after the answer = %+v", status)
	}
}

// A task that waited out its life with a grant fails without asking the
// harness whether it is idle.
func TestExpiredGrantFailsWithoutAReservation(t *testing.T) {
	dir := stateDir(t)
	m := message("old task")
	m.Kind, m.FromEpoch, m.ToEpoch, m.GrantGit = Task, "sender-epoch", "receiver-epoch", true
	m.CreatedAt = time.Now().Add(-2 * DefaultTTL)
	if err := Put(dir, m); err != nil {
		t.Fatal(err)
	}
	s := batchServer(dir)
	s.Reserve = func(context.Context, Message) (Reservation, error) {
		t.Fatal("an expired grant was reserved")
		return nil, nil
	}
	s.drain(context.Background())
	if status, ok := ReadStatus(dir, "api", m.ID); !ok || status.State != Failed {
		t.Fatalf("status = %+v", status)
	}
}

func TestSettledFollowsTheTaskToItsReport(t *testing.T) {
	dir := stateDir(t)
	m := grantPending(t, dir, true)[0]
	if Settled(dir, "api", "receiver-epoch", m.ID) {
		t.Fatal("a task waiting in the inbox is settled")
	}
	if err := state.WithMailboxLock(context.Background(), dir, "api", func() error {
		if err := linkUnread(dir, "api", m.ID); err != nil {
			return err
		}
		return MarkRead(dir, "api", "receiver-epoch", m, true)
	}); err != nil {
		t.Fatal(err)
	}
	if Settled(dir, "api", "receiver-epoch", m.ID) {
		t.Fatal("a task read and not reported on is settled")
	}
	for _, waiter := range Waiters(dir, "api", "receiver-epoch") {
		ClearAwaiting(dir, "api", "receiver-epoch", waiter)
	}
	if !Settled(dir, "api", "receiver-epoch", m.ID) {
		t.Fatal("a task reported on is not settled")
	}

	// Swept: no status and no copy ahead of the reader.
	other := grantPending(t, dir, true)[0]
	if err := os.Remove(filepath.Join(state.InboxPath(dir, "api"), other.ID+".json")); err != nil {
		t.Fatal(err)
	}
	if !Settled(dir, "api", "receiver-epoch", other.ID) {
		t.Fatal("a swept task is not settled")
	}
}
