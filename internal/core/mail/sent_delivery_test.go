package mail

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// awaitAnswers makes the wait answer status, known or not, after then ran:
// what the serving process did while the sender waited.
func awaitAnswers(t *testing.T, status Status, known bool, then func(dir, to, id string)) {
	t.Helper()
	previous := await
	t.Cleanup(func() { await = previous })
	await = func(dir, to, id string, _ time.Duration, _ func() bool) (Status, bool) {
		if then != nil {
			then(dir, to, id)
		}
		return status, known
	}
}

// What a sender is told of a letter just written follows from its status,
// the mailbox and the registry, each looked at once more after the wait.
func TestAwaitSentSaysWhatBecameOfTheLetter(t *testing.T) {
	none := func(string, string, string) {}
	writeStatus := func(state State, detail string) func(dir, to, id string) {
		return func(dir, to, id string) {
			if err := WriteStatus(dir, to, id, Result{State: state, Via: "socket", Detail: detail}); err != nil {
				t.Fatal(err)
			}
		}
	}
	gone := func(dir, to, id string) {
		if err := state.Remove(filepath.Join(state.InboxPath(dir, to), id+".json")); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name   string
		status Status
		known  bool
		then   func(dir, to, id string)
		target func(registry.Session) registry.Session
		want   Delivery
	}{
		{name: "delivered", status: Status{State: Delivered, Via: "socket"}, known: true, then: none, want: Delivery{State: Delivered, Via: "socket"}},
		// The status landed after the wait gave up, and the letter left the
		// mailbox: a fresh delivery, not a lost one.
		{name: "late", then: func(dir, to, id string) { writeStatus(Delivered, "")(dir, to, id); gone(dir, to, id) }, want: Delivery{State: Delivered, Via: "socket"}},
		{name: "read", status: Status{State: Read, Via: "socket"}, known: true, then: none, want: Delivery{State: Delivered, Fate: FateRead, Via: "socket"}},
		{name: "held", status: Status{State: Held, Via: "socket", Detail: "queued"}, known: true, then: none, want: Delivery{State: Held, Via: "socket", Detail: "queued"}},
		{name: "waiting", then: writeStatus(Pending, "compacting"), want: Delivery{State: Pending, Detail: "compacting"}},
		{name: "no result", then: none, want: Delivery{State: Pending, Fate: FateNoResult}},
		{name: "answered long ago", then: gone, want: Delivery{State: Failed, Fate: FateAnsweredLongAgo}},
		{name: "ended", then: func(dir, _, _ string) {
			if err := os.Remove(state.SessionPath(dir, "api")); err != nil {
				t.Fatal(err)
			}
		}, want: Delivery{State: Failed, Fate: FateEnded}},
		{name: "replaced", then: none, target: func(s registry.Session) registry.Session { s.ServiceStart++; return s }, want: Delivery{State: Failed, Fate: FateReplaced}},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newSentLab(t)
			letter := lab.send(t, Task, "rerun the smoke", "")
			awaitAnswers(t, c.status, c.known, c.then)
			target := lab.api
			if c.target != nil {
				target = c.target(target)
			}
			if got := AwaitSent(lab.dir, target, letter.ID, time.Second); got != c.want {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

// A status that cannot be read may say delivered or read: it is pending,
// with why, and never failed, whoever holds the name now.
func TestAnUnreadableStatusIsPending(t *testing.T) {
	lab := newSentLab(t)
	letter := lab.send(t, Task, "rerun the smoke", "")
	awaitAnswers(t, Status{}, false, func(dir, to, id string) {
		unreadable(t, filepath.Join(state.InboxPath(dir, to), id+".status"), []byte(`{"state":"read"}`))
	})
	ended := lab.api
	ended.ServiceStart++
	if got := AwaitSent(lab.dir, ended, letter.ID, time.Second); got.State != Pending || got.Fate != FateUnknown || got.Err == nil {
		t.Fatalf("an unreadable status answered %+v", got)
	}
}
