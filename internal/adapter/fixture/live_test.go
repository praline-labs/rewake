//go:build rewaketest

package fixture

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
)

// heardLive records each live set the run reported, in order.
type heardLive struct {
	mu   sync.Mutex
	sets [][]string
}

func (h *heardLive) report(set []adapter.Capability) {
	names := []string{}
	for _, capability := range set {
		names = append(names, string(capability))
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sets = append(h.sets, names)
}

func (h *heardLive) all() [][]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.sets)
}

// The host hears the live set once the first exchange has run, never
// before, then the withdrawal at once and the second exchange's set: the
// whole set each time.
func TestTheRunReportsItsLiveSetAsItChanges(t *testing.T) {
	heard := &heardLive{}
	b, err := startConfigured(t, func(b *backend) { b.ReportLive(heard.report) }, switchDrop+"=1", switchAgain+"=1")
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "the second exchange", func() bool { return len(heard.all()) >= 3 })
	got := heard.all()
	if !slices.Equal(got[0], Served) || len(got[1]) != 0 || !slices.Equal(got[2], Served) {
		t.Fatalf("the reports: %q", got)
	}
	if !slices.Equal(live(b), Served) {
		t.Fatalf("live after the reconnect: %v", live(b))
	}
}

// A capability the exchange did not prove is absent from the first report.
func TestTheFirstReportLeavesOutWhatTheExchangeDidNotProve(t *testing.T) {
	heard := &heardLive{}
	if _, err := startConfigured(t, func(b *backend) { b.ReportLive(heard.report) }, switchNo+"="+TurnBoundary); err != nil {
		t.Fatal(err)
	}
	got := heard.all()
	if len(got) != 1 || slices.Contains(got[0], TurnBoundary) || !slices.Contains(got[0], Wake) {
		t.Fatalf("the reports: %q", got)
	}
}

// The briefing is handed over once, after the exchange.
func TestTheBriefingIsHandedOverOnce(t *testing.T) {
	file := filepath.Join(buildtest.Dir(t), "brief")
	calls := 0
	setup := func(b *backend) {
		b.brief = func() string { calls++; return "the briefing" }
	}
	if _, err := startConfigured(t, setup, switchBrief+"="+file); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(file)
	if err != nil || string(raw) != "the briefing" || calls != 1 {
		t.Fatalf("the briefing %q (%v), built %d times", raw, err, calls)
	}
}

// A request in the run's control directory reaches the program while Control
// is live, and its answer comes back; without a live Control nothing is
// carried.
func TestControlIsCarriedOnlyWhileLive(t *testing.T) {
	limits := control.Limits{Pickup: 3 * time.Second, Outcome: 3 * time.Second, Poll: 10 * time.Millisecond}
	for _, withControl := range []bool{true, false} {
		t.Run(strconv.FormatBool(withControl), func(t *testing.T) {
			dir := filepath.Join(buildtest.Dir(t), "control")
			if err := control.Prepare(dir); err != nil {
				t.Fatal(err)
			}
			var switches []string
			if !withControl {
				switches = append(switches, switchNo+"="+Control)
			}
			if _, err := startConfigured(t, func(b *backend) { b.controlDir = dir }, switches...); err != nil {
				t.Fatal(err)
			}
			answer, err := control.Ask(context.Background(), dir, control.Request{ID: control.NewID(), Action: control.Interrupt, From: "lead"}, limits)
			if err != nil {
				t.Fatal(err)
			}
			want := control.Answer{Outcome: control.Refused, Reason: control.NotAnswering}
			if withControl {
				want = control.Answer{Outcome: "done", Reason: "interrupt from lead"}
			}
			if answer.Outcome != want.Outcome || answer.Reason != want.Reason {
				t.Fatalf("the answer %+v, want %+v", answer, want)
			}
		})
	}
}
