package host

import (
	"context"
	"slices"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// reasonsBoundary is a run whose TurnBoundary reports one end of each reason,
// published and then confirmed, once its service starts.
type reasonsBoundary struct {
	handler adapter.TurnHandler
	errs    []error
}

var (
	_ adapter.TurnBoundary = (*reasonsBoundary)(nil)
	_ adapter.Service      = (*reasonsBoundary)(nil)
)

var everyReason = []adapter.EndReason{adapter.Answered, adapter.Aborted, adapter.Refused, adapter.Failed}

func (b *reasonsBoundary) ReportTurns(handler adapter.TurnHandler) { b.handler = handler }

func (b *reasonsBoundary) Start(ctx context.Context, _ func(string)) error {
	for _, reason := range everyReason {
		end := adapter.TurnEnd{Event: "published/" + string(reason), Reason: reason, Answer: "words", Boundary: b.handler.Capture()}
		b.errs = append(b.errs, b.handler.Publish(ctx, end))
		end.Event = "confirmed/" + string(reason)
		_, err := b.handler.Confirm(ctx, end)
		b.errs = append(b.errs, err)
	}
	return nil
}

func (*reasonsBoundary) Close() {}

type reasonsHarness struct {
	fakeHarness
	run *reasonsBoundary
}

func (h *reasonsHarness) Launch(request adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	plan, err := h.fakeHarness.Launch(request)
	plan.Service, plan.TurnBoundary = h.run, h.run
	return plan, err
}

// The wrapper hands a TurnBoundary the handler before its run starts, and
// passes each end on as the adapter reported it — its event and its reason —
// to the publication or to the confirmation, with a scope captured from the
// run's read clock: what the end reports is the core's to decide (O2), never
// the wrapper's.
func TestTheWrapperPassesEachEndOnWithItsEventAndReason(t *testing.T) {
	dir := stateDir(t)
	run := &reasonsBoundary{}
	fake := &reasonsHarness{fakeHarness: fakeHarness{script: "exit 0"}, run: run}
	var published, confirmed []adapter.TurnEnd
	_, err := Run(context.Background(), Request{
		Harness: fake, Dir: dir, Name: "api",
		OnTurn: func(_ context.Context, _ registry.Session, end adapter.TurnEnd) error {
			published = append(published, end)
			return nil
		},
		OnConfirm: func(_ context.Context, _ registry.Session, end adapter.TurnEnd) (string, error) {
			confirmed = append(confirmed, end)
			return "", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(run.errs, func(err error) bool { return err != nil }) {
		t.Fatalf("the wrapper refused an end: %v", run.errs)
	}
	for kind, ends := range map[string][]adapter.TurnEnd{"published": published, "confirmed": confirmed} {
		if len(ends) != len(everyReason) {
			t.Fatalf("%s %d ends of %d", kind, len(ends), len(everyReason))
		}
		for i, end := range ends {
			reason := everyReason[i]
			if end.Event != kind+"/"+string(reason) || end.Reason != reason || end.Answer != "words" || end.Boundary == nil {
				t.Fatalf("%s end %d arrived as %+v", kind, i, end)
			}
		}
	}
}
