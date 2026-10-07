package workflow

// The columns a scenario runs in, and what each of them can show.
//
// The scenarios are the same text in each: the same observations, the same
// controls. What differs is the evidence a harness makes available, and that
// difference is named rather than worked around. A column that cannot show
// something records the observation as unsupported with the capability it
// lacks — check-runner.md is explicit that a capability applies to a single
// observation as well as to a whole case, and that unsupported is not a pass.
//
// They are not equal in what a red result means, either. Codex is the
// regression gate and Claude Code is the search column, because that adapter
// is younger and less exercised; the fixture runs beside them until stage 3
// makes it the gate (docs/v2/stage3-fixture.md#the-gate-across-the-steps). The
// summary names the column of every red and nothing here promotes one on its
// own.

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

// knownCapabilities is every capability any scenario asks about. It is filled
// by declaring one, never written by hand: a list maintained beside the
// declarations would be the next thing to drift, and the test that holds the
// gate column to every capability would then hold it to a stale list.
var knownCapabilities []string

// capability declares one thing a fixture can show, and registers it.
func capability(name string) string {
	knownCapabilities = append(knownCapabilities, name)
	return name
}

// Each capability traces to a row of the feature map, and each is absent for
// a reason that belongs to the harness rather than to the fixture.
var (
	// capabilitySelection: the session reports which conversation it has
	// selected and accepted, which is the Codex server's selection fencing.
	// The socket column learns the conversation only from hooks after the
	// fact — one line goes in, nothing comes back — so there is no selection
	// to report.
	capabilitySelection = capability("reports-conversation-selection")
	// capabilityNamesMembers: a delivery names the messages it carries. The
	// socket column's notification carries an id for the announcement, a
	// count and a preview, and never the member ids.
	capabilityNamesMembers = capability("names-delivered-message-ids")
)

// column is one harness fixture: the name rewake launches it by, and what it
// can show.
type column struct {
	harness string
	caps    map[string]bool
}

func (col column) offers(name string) bool { return col.caps[name] }

// isGate reports whether a case ran on the column whose red blocks. Codex is
// that column: the path that must not break, where every observation has to
// be made. The other column searches, and a capability absent there is a fact
// about a younger harness rather than a hole in the gate.
func isGate(harness string) bool { return harness == codexColumn.harness }

// The columns the suite runs. mid-turn's capability is declared here too, so
// one table answers every question about what a column can show.
var (
	codexColumn = column{harness: "codex", caps: map[string]bool{
		capabilitySelection:    true,
		capabilityNamesMembers: true,
		capabilityMidTurn:      true,
	}}
	claudeColumn = column{harness: "claude", caps: map[string]bool{}}
	// fixtureColumn is the harness the core is proven on without a real one
	// (docs/v2/stage3-fixture.md): it names its members and steers into a
	// running turn; it reports no selection, which is Codex's alone.
	fixtureColumn = column{harness: "fixture", caps: map[string]bool{
		capabilityNamesMembers: true,
		capabilityMidTurn:      true,
	}}
)

// unsupported records an observation this column cannot make, by name and with
// the capability it needs. Saying so is the point: a missing mechanism that
// stayed silent would read as a defect that swallowed the evidence.
func (col column) unsupported(c *Case, observation, name string) {
	c.UnsupportedCapability(observation, name, col.harness+" declares no "+name)
}

// noticeID is the id a socket-column delivery carries: the tag the adapter
// builds from the last eight characters of the message id. Spelled out here
// rather than imported, like everything else this fixture checks the product
// against.
func noticeID(id string) string {
	if len(id) <= 8 {
		return "rewake-" + id
	}
	return "rewake-" + id[len(id)-8:]
}

// deliveryNamed reports whether a delivery this session recorded named one
// message. The two columns name a message differently — one by its id, the
// other by the tag an announcement carries — and both are the delivery's own
// word about what it carried.
func (col column) deliveryNamed(worker *codexSession, id string) bool {
	named := worker.deliveredIDs()
	if col.offers(capabilityNamesMembers) {
		return slices.Contains(named, id)
	}
	return slices.Contains(named, noticeID(id))
}

// errRecordBehind is a recipient's record caught between two of its own
// writes: the fixture records a delivery and then the overview it read for
// it, and a check that lands in between sees one more delivery than
// overviews. A waiting caller waits on; a judgement made after its anchor
// still fails on it.
var errRecordBehind = errors.New("the recipient's record is still being written")

// announcedDeliveries are the deliveries a session recorded, each with the
// messages it announced.
//
// On the column whose notification names its members, that is what was
// recorded. On the other it is reconstructed from the session's own overviews:
// the mail available at the nth delivery, less the mail that was already
// available at the one before, is what the nth delivery announced. The
// reconstruction is checked against the count the notice carried — a
// membership that does not have the announced size is a contradiction, not a
// detail — so the answer is either both records agreeing or an error.
func (col column) announcedDeliveries(worker *codexSession) ([]groupDelivery, error) {
	deliveries, err := worker.groupDeliveries()
	if err != nil {
		return nil, err
	}
	if col.offers(capabilityNamesMembers) {
		// A column that names its members has to have named them. A delivery
		// with fewer names than it announced is not a shorter group; it is a
		// record that lost them — and no-replay would then hold on nothing.
		for _, delivery := range deliveries {
			if len(delivery.Members) != delivery.Count {
				return nil, fmt.Errorf("%s announced %d messages and named %d", delivery.Turn, delivery.Count, len(delivery.Members))
			}
		}
		return deliveries, nil
	}
	overviews, err := worker.overviews()
	if err != nil {
		return nil, err
	}
	if len(overviews) < len(deliveries) {
		return nil, fmt.Errorf("%w: the recipient recorded %d deliveries and %d overviews, so what each announced cannot be reconstructed",
			errRecordBehind, len(deliveries), len(overviews))
	}
	var before []string
	for index := range deliveries {
		available := overviews[index]
		var fresh []string
		for _, id := range available {
			if !slices.Contains(before, id) {
				fresh = append(fresh, id)
			}
		}
		// Two readings, and the count decides between them. Ordinarily a
		// delivery announces what newly became available. Where something
		// already announced is announced again, it announces everything still
		// waiting — so when that whole set has the announced size and the
		// fresh one does not, that is what it named. A count matching neither
		// is a record this column cannot resolve, and saying so beats picking.
		switch {
		case len(fresh) == deliveries[index].Count:
			deliveries[index].Members = fresh
		case len(available) == deliveries[index].Count:
			deliveries[index].Members = available
		default:
			return nil, fmt.Errorf("%s announced %d messages while %d were waiting and %d were new",
				deliveries[index].Turn, deliveries[index].Count, len(available), len(fresh))
		}
		before = append(before, fresh...)
	}
	return deliveries, nil
}

// replayedAnnouncement names the first thing this session was told about
// twice, or "" when nothing was.
//
// The two columns show a replay differently, and the difference is not a
// detail. Where a delivery names its members, a replay names one of them
// again. Where it does not, the announcement carries an id derived from the
// members — so a group rebuilt around a replayed letter and a new one carries
// a *different* id, and comparing ids would miss it. What that column shows
// instead is arithmetic: a delivery announcing more messages than newly became
// available has announced something that was already announced.
func (col column) replayedAnnouncement(worker *codexSession) (string, error) {
	deliveries, err := worker.groupDeliveries()
	if err != nil {
		return "", err
	}
	if col.offers(capabilityNamesMembers) {
		for _, delivery := range deliveries {
			if len(delivery.Members) != delivery.Count {
				return "", fmt.Errorf("%s announced %d messages and named %d, so a replay cannot be judged",
					delivery.Turn, delivery.Count, len(delivery.Members))
			}
		}
		return announcedTwice(deliveries), nil
	}
	overviews, err := worker.overviews()
	if err != nil {
		return "", err
	}
	if len(overviews) < len(deliveries) {
		return "", fmt.Errorf("the recipient recorded %d deliveries and %d overviews, so a replay cannot be judged",
			len(deliveries), len(overviews))
	}
	var before []string
	for index, delivery := range deliveries {
		fresh := 0
		for _, id := range overviews[index] {
			if !slices.Contains(before, id) {
				fresh++
				before = append(before, id)
			}
		}
		if delivery.Count > fresh {
			return fmt.Sprintf("%s announced %d messages while %d became newly available",
				delivery.Turn, delivery.Count, fresh), nil
		}
	}
	return "", nil
}

// runInColumns runs one scenario body in every column, under a subtest each.
// The scenario keeps one name: the column is a property of the case, which is
// what the summary prints beside it, not a second scenario.
//
// The test itself is made parallel so that its columns, each of which joins
// the pool, can overlap other tests and not only each other.
func runInColumns(t *testing.T, name string, body func(t *testing.T, col column)) {
	t.Helper()
	runParallel(t)
	for _, col := range []column{codexColumn, claudeColumn, fixtureColumn} {
		t.Run(col.harness, func(t *testing.T) { body(t, col) })
	}
	_ = name
}

// readinessSwitch is how a sender waits for its recipient on this column. One
// reports an accepted conversation in the room's telemetry; the other has none
// and says it is listening by creating a file. Both are the recipient's own
// word, read before the first letter leaves.
func readinessSwitch(col column, recipient *codexSession) string {
	if col.offers(capabilitySelection) {
		return shimSendWhenReady + "=" + recipient.name
	}
	return shimWaitForFile + "=" + recipient.ready
}
