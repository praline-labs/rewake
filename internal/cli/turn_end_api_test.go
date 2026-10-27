package cli

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// The turn end as the adapter API carries it (adapter.TurnEnd), on both paths a
// TurnBoundary hands one to the core: published, and confirmed by an adapter
// that can hold it.

type endPath func(context.Context, string, registry.Session, adapter.TurnEnd) error

func endPaths() map[string]endPath {
	return map[string]endPath{
		"published": func(ctx context.Context, dir string, self registry.Session, end adapter.TurnEnd) error {
			_, err := ReportCompletion(ctx, dir, self, end)
			return err
		},
		"confirmed": func(ctx context.Context, dir string, self registry.Session, end adapter.TurnEnd) error {
			_, err := ConfirmCompletion(ctx, dir, self, end)
			return err
		},
	}
}

// Every end names its event and carries a reason the core knows: one that does
// not is refused on either path, reports nothing, and its waits stay owed for
// the next end (E7; an end's reason is never guessed, O2).
func TestATurnEndWithoutItsEventOrAKnownReasonIsRefused(t *testing.T) {
	ends := map[string]func(*mail.ReadBoundary) adapter.TurnEnd{
		"no boundary": func(*mail.ReadBoundary) adapter.TurnEnd {
			return adapter.TurnEnd{Event: "run/A/1", Thread: "A", Reason: adapter.Answered, Answer: "done"}
		},
		"no event": func(b *mail.ReadBoundary) adapter.TurnEnd {
			return adapter.TurnEnd{Thread: "A", Reason: adapter.Answered, Answer: "done", Boundary: b}
		},
		"no reason": func(b *mail.ReadBoundary) adapter.TurnEnd {
			return adapter.TurnEnd{Event: "run/A/1", Thread: "A", Answer: "done", Boundary: b}
		},
		"an unknown reason": func(b *mail.ReadBoundary) adapter.TurnEnd {
			return adapter.TurnEnd{Event: "run/A/1", Thread: "A", Reason: "completed", Answer: "done", Boundary: b}
		},
	}
	for path, end := range endPaths() {
		for name, build := range ends {
			t.Run(path+"/"+name, func(t *testing.T) {
				dir := failedWaiters(t)
				self := selfOf(t, dir)
				if err := end(context.Background(), dir, self, build(boundaryNow(t, dir, self))); err == nil {
					t.Fatal("the end was taken")
				}
				for _, peer := range []string{"one", "two"} {
					if files := finishedFor(t, dir, peer); len(files) != 0 {
						t.Fatalf("reports to %s: %v", peer, files)
					}
				}
				if len(mail.Open(dir, "api", self.Epoch()).Waiters()) != 2 {
					t.Fatal("the refused end cleared its waits")
				}
			})
		}
	}
}

// What an end reports follows from the reason the adapter's end event gave,
// on either path: an answered turn reports finished with its answer, an
// aborted one stopped, a failed or refused one an error with the harness's
// words unchanged (O2).
func TestEachEndReasonReportsItsOutcome(t *testing.T) {
	want := map[adapter.EndReason]mail.Kind{
		adapter.Answered: mail.Finished,
		adapter.Aborted:  mail.Stopped,
		adapter.Failed:   mail.Error,
		adapter.Refused:  mail.Error,
	}
	for path, end := range endPaths() {
		for reason, kind := range want {
			t.Run(path+"/"+string(reason), func(t *testing.T) {
				dir := failedWaiters(t)
				self := selfOf(t, dir)
				words := "the harness's own words for " + string(reason)
				taken := adapter.TurnEnd{Event: "run/A/" + string(reason), Thread: "A", Reason: reason, Answer: words, Boundary: boundaryNow(t, dir, self)}
				if err := end(context.Background(), dir, self, taken); err != nil {
					t.Fatal(err)
				}
				for _, peer := range []string{"one", "two"} {
					report := reportObject(t, dir, peer)
					if report["kind"] != string(kind) || report["text"] != words {
						t.Fatalf("%s reported to %s as %v: %v", reason, peer, report["kind"], report)
					}
				}
			})
		}
	}
}
