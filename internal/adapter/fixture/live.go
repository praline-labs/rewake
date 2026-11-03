//go:build rewaketest

package fixture

import (
	"slices"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
)

// ReportLive takes where the run's live set goes, before Start
// (adapter.LiveReporter).
func (b *backend) ReportLive(live func([]adapter.Capability)) { b.report = live }

// changed tells the host what is live now, after every change of it, once the
// first exchange has run. A withdrawal is told at once, the set empty. Outside
// b.mu, which a call into the backend takes; under changes, so the calls keep
// the order of the changes they tell.
func (b *backend) changed() {
	b.changes.Lock()
	defer b.changes.Unlock()
	b.mu.Lock()
	live, report := b.current(), b.report
	if !b.exchanged {
		report = nil
	}
	b.mu.Unlock()
	if report != nil {
		report(live)
	}
}

// briefBound is how long the program has to take the briefing.
const briefBound = 2 * time.Second

// sendBrief hands the program its briefing once the first exchange has run.
func (b *backend) sendBrief(l *link) {
	if b.brief == nil {
		return
	}
	_, _ = l.ask(Frame{Op: opBrief, Text: b.brief()}, briefBound)
}

// probe sends one probe per served capability, all at once, and answers the
// set whose probes came back right within the bound, and the activity the
// Telemetry probe's answer carried, if any. A capability the hello did not
// name is not probed.
func (b *backend) probe(l *link, serves []string) (map[string]bool, string) {
	live := map[string]bool{}
	var activity string
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, capability := range Served {
		if !slices.Contains(serves, capability) {
			continue
		}
		request := Frame{Op: opProbe, Capability: capability}
		wg.Add(1)
		go func() {
			defer wg.Done()
			answer, err := l.ask(request, b.bound)
			if err == nil && probeAnswered(capability, answer) {
				mu.Lock()
				live[capability] = true
				if capability == Telemetry && answer.State != "none" {
					activity = answer.State
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return live, activity
}

// probeAnswered says whether a probe's answer is the one its capability
// gives: an acknowledgment for Wake, a turn state for TurnBoundary, a sample
// or none for Telemetry, ready for Control.
func probeAnswered(capability string, answer Frame) bool {
	if !answer.OK {
		return false
	}
	switch capability {
	case TurnBoundary:
		return answer.State == "idle" || answer.Turn != "" && answer.State == "turn"
	case Telemetry:
		return answer.State == "none" || answer.State == "working" || answer.State == "idle"
	case Control:
		return answer.State == "ready"
	}
	return true
}

// current is the live set now: the capabilities whose probes the program
// answered on the connection held now, and nothing else. The caller holds
// b.mu.
func (b *backend) current() []adapter.Capability {
	var out []adapter.Capability
	for _, capability := range Served {
		if b.live[capability] {
			out = append(out, adapter.Capability(capability))
		}
	}
	// A space host is rewake's own process, which needs of the run only that
	// it runs: the program's connection proves that.
	if b.hostsSpaces && b.link != nil {
		out = append(out, adapter.SpacesCapability)
	}
	return out
}

func (b *backend) isLive(capability string) (*link, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.link, b.link != nil && b.live[capability]
}
