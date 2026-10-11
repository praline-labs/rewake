//go:build rewakefixture

package fixture

import (
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
)

// ReportLive takes where the run's live set goes, before Start
// (adapter.LiveReporter).
func (b *backend) ReportLive(live func([]adapter.Capability)) { b.report = live }

// changed tells what is live now, after every change of it: the endpoint which
// process it serves (toolsLive), then the host the whole set, once the first
// exchange has run. A withdrawal is told at once, the set empty. Outside b.mu,
// which a call into the backend takes; under changes, so the calls keep the
// order of the changes they tell.
func (b *backend) changed() {
	b.changes.Lock()
	defer b.changes.Unlock()
	b.toolsLive()
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

// sendBrief hands the program its briefing, built once the first exchange
// has shown whether the tool is live: the briefing then tells the session
// how to reach the mail as this run can, the tool's whole sentence or the
// shell's alone (brief.Context.Tool).
func (b *backend) sendBrief(l *link, tool bool) {
	if b.brief == nil {
		return
	}
	_, _ = l.ask(Frame{Op: opBrief, Text: b.brief(tool)}, briefBound)
}
