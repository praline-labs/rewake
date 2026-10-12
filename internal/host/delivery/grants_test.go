package delivery

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
)

func ids(messages []mail.Message) []string {
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
	s.CheckGrant = func(mail.Message) error { return nil }
	idle := false
	var notices [][]string
	s.Reserve = func(_ context.Context, m mail.Message) (mail.Reservation, error) {
		if mail.CarriesGrant(m) && !idle {
			return nil, fmt.Errorf("%w: waiting for idle", mail.ErrNotYet)
		}
		r := &reservationFixture{}
		r.prepare = func(fn func(string) error) error { return fn("root-A") }
		r.deliver = func(_ context.Context, m mail.Message) mail.Result {
			batch := m.Batch
			if len(batch) == 0 {
				batch = []mail.Message{m}
			}
			notices = append(notices, ids(batch))
			return mail.Result{State: mail.Delivered}
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
	if available, _ := mail.AvailableUnread(dir, "api", s.Epoch); slices.Contains(ids(available), members[1].ID) {
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
	for name, check := range map[string]func(mail.Message) error{
		"changed":       func(mail.Message) error { return errors.New("/work/lib now leads to /etc: a link changed") },
		"not confirmed": func(mail.Message) error { return errors.New("main did not confirm the grant") },
		"no check":      nil,
	} {
		t.Run(name, func(t *testing.T) {
			dir := stateDir(t)
			members := grantPending(t, dir, true)
			s := batchServer(dir)
			s.CheckGrant = check
			s.Reserve = func(context.Context, mail.Message) (mail.Reservation, error) {
				t.Fatal("a failed grant was reserved")
				return nil, nil
			}
			s.drain(context.Background())
			status, ok, _ := mail.ReadStatus(dir, "api", members[0].ID)
			if !ok || status.State != mail.Failed || !strings.Contains(status.Detail, "does not pass") {
				t.Fatalf("status = %+v", status)
			}
			told, err := mail.List(dir, "web")
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
	s.Deliver = func(context.Context, mail.Message) mail.Result { return mail.Result{State: mail.Delivered} }
	answered := false
	s.CheckGrant = func(mail.Message) error {
		if !answered {
			return fmt.Errorf("%w: the wrapper of web did not confirm the grant yet", mail.ErrNotYet)
		}
		return nil
	}
	s.drain(context.Background())
	if status, ok, _ := mail.ReadStatus(dir, "api", members[0].ID); !ok || status.State != mail.Pending || !strings.Contains(status.Detail, "did not confirm the grant yet") {
		t.Fatalf("status = %+v", status)
	}
	answered, s.attempts = true, map[string]time.Time{}
	s.drain(context.Background())
	if status, ok, _ := mail.ReadStatus(dir, "api", members[0].ID); !ok || status.State != mail.Delivered {
		t.Fatalf("status after the answer = %+v", status)
	}
}

// A task that waited out its life with a grant fails without asking the
// harness whether it is idle.
func TestExpiredGrantFailsWithoutAReservation(t *testing.T) {
	dir := stateDir(t)
	m := message("old task")
	m.Kind, m.FromEpoch, m.ToEpoch, m.GrantGit = mail.Task, "sender-epoch", "receiver-epoch", true
	m.CreatedAt = time.Now().Add(-2 * DefaultTTL)
	if err := mail.Put(dir, m); err != nil {
		t.Fatal(err)
	}
	s := batchServer(dir)
	s.Reserve = func(context.Context, mail.Message) (mail.Reservation, error) {
		t.Fatal("an expired grant was reserved")
		return nil, nil
	}
	s.drain(context.Background())
	if status, ok, _ := mail.ReadStatus(dir, "api", m.ID); !ok || status.State != mail.Failed {
		t.Fatalf("status = %+v", status)
	}
}
