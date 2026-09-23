package workflow

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestBatchArrival is the grouping scenario: two letters that arrive inside
// one collection window are announced as one group of fixed membership, a
// letter after the window belongs to another group, and reads stay per
// message — an overview consumes nothing, one read takes one letter, and what
// was announced is never announced again.
//
// The last claim is the one an empty scenario would prove by itself: with
// nothing announced and unread, there is nothing to replay. So the recipient
// leaves the last member of a group unread until the next delivery, and the
// third letter comes late enough for a replay to have shown in between.
//
// The controls break the product, not the fixture: each one is rewake with a
// single line changed, built through an overlay (mutant_test.go).
func TestBatchArrival(t *testing.T) {
	runInColumns(t, "batch-arrival", runBatchArrival)
}

func runBatchArrival(t *testing.T, col column) {
	binary := enterScenario(t, "batch-arrival")
	c := Start(t, Spec{
		Name:    "batch-arrival",
		Harness: col.harness,
		Observations: []string{
			obsReady, obsListening, obsGroupOfTwo, obsPreviewLatest, obsThirdOutside,
			obsPeekConsumedNothing, obsReadOneByOne, obsNoReplay, obsAlive,
		},
		Deadline: 90 * time.Second,
	})
	iso := Isolate(t, c, binary)
	worker, sender := startBatchSessions(t, c, iso, col)
	defer stopSession(t, c, worker)
	defer stopSession(t, c, sender)

	if !awaitBatchReady(c, col, worker, sender) {
		return
	}

	// The ids are learnt from the recipient's own overview, not from the
	// sender: the claim is about what the recipient was told.
	alpha, beta := awaitFirstTwo(c, worker)
	group := deliveryContaining(c, col, worker, alpha.ID)
	members := append([]string(nil), group.Members...)
	slices.Sort(members)
	if !slices.Equal(members, sorted(alpha.ID, beta.ID)) {
		c.Contradicted(obsGroupOfTwo, "the delivery carrying %s named %v, not exactly the two close letters", alpha.ID, group.Members)
	} else {
		c.Observed(obsGroupOfTwo, fmt.Sprintf("one delivery named exactly %v", group.Members))
	}

	latest, earliest := latestOf(alpha, beta)
	switch {
	case strings.Contains(group.Notice, latest.Preview) && !strings.Contains(group.Notice, earliest.Preview):
		c.Observed(obsPreviewLatest, "the notice previews "+latest.Preview)
	default:
		c.Contradicted(obsPreviewLatest, "the notice %q should preview %s, the latest of the two", group.Notice, latest.Preview)
	}

	gamma := awaitThird(c, worker)
	third := deliveryContaining(c, col, worker, gamma.ID)
	if !slices.Equal(third.Members, []string{gamma.ID}) || third.Turn == group.Turn {
		c.Contradicted(obsThirdOutside, "the third letter %s arrived as %v in %s, the first group was %s", gamma.ID, third.Members, third.Turn, group.Turn)
	} else {
		c.Observed(obsThirdOutside, "announced alone, after the group")
	}

	// Anchored on the recipient's own record of its reads: every letter has
	// been attempted once the third has, because the member deferred from the
	// first group is read at the same delivery, just before it.
	c.Await("the recipient to attempt every letter", func() bool {
		for _, id := range []string{alpha.ID, beta.ID, gamma.ID} {
			if _, attempted := readOf(worker, id); !attempted {
				return false
			}
		}
		return true
	})
	judgeReads(c, worker, alpha.ID, beta.ID, gamma.ID)
	judgeNoReplay(c, col, worker)

	if !worker.alive() || !sender.alive() {
		c.Contradicted(obsAlive, "worker alive: %v, sender alive: %v", worker.alive(), sender.alive())
		return
	}
	c.Observed(obsAlive, "neither session left before the verdict")
}

// The observations, named once: the scenario records them and the controls
// name which of them their breakage must take down.
const (
	obsReady               = "the recipient reaches an accepted conversation"
	obsListening           = "the recipient can receive mail before the letters leave"
	obsGroupOfTwo          = "two close letters are announced as one group of two"
	obsPreviewLatest       = "the notice previews the latest member"
	obsThirdOutside        = "the third letter is announced outside that group"
	obsPeekConsumedNothing = "an overview consumed nothing"
	obsReadOneByOne        = "each member is read on its own"
	obsNoReplay            = "announced mail is never announced again"
	obsAlive               = "both sessions are still running when the case is judged"
)

// The three letters, told apart by text. The sender puts the first two in
// one window and the third laterSendDelay after them.
const (
	batchAlpha = "batch-arrival-alpha"
	batchBeta  = "batch-arrival-beta"
	batchGamma = "batch-arrival-gamma"
)

// startBatchSessions launches the recipient, which reads one letter at a
// time, and the sender, which waits for the recipient to be ready before its
// letters leave — mail sent earlier would wait in the mailbox and be folded
// into the first group, which would measure readiness rather than the window.
func startBatchSessions(t *testing.T, c *Case, iso *Isolation, col column, controls ...string) (*codexSession, *codexSession) {
	t.Helper()
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general",
		append([]string{shimReadEach + "=1"}, controls...)...)
	sender := startHarnessSession(t, c, iso, col.harness, "sender", "--main",
		shimSendTo+"="+worker.name, readinessSwitch(col, worker),
		shimSendTexts+"="+batchAlpha+"|"+batchBeta, shimSendLaterText+"="+batchGamma)
	return worker, sender
}

// awaitBatchReady waits for whatever readiness this column has, and records
// the observation it can support. One column reports an accepted conversation;
// the other has none, and says so by name — while still waiting for the
// recipient to be listening, which on that column is a file it writes itself.
func awaitBatchReady(c *Case, col column, worker, sender *codexSession) bool {
	if !col.offers(capabilitySelection) {
		col.unsupported(c, obsReady, capabilitySelection)
		// No conversation here, but the recipient still has to be able to
		// receive before the letters leave, and it says so by creating a file.
		// A red line about that names this, not the grouping that never got a
		// chance to happen.
		if !waitFor(c, batchWindow, func() bool { return exists(worker.ready) }) {
			c.Contradicted(obsListening, "%s never started listening", worker.name)
			return false
		}
		c.Observed(obsListening, "the recipient's socket is listening")
		return true
	}
	if _, ok := sender.await(c, "a selection for "+worker.name, func(l listing) bool {
		_, selection, _, found := l.find(worker.name)
		return found && selection == "ready"
	}); !ok {
		c.Contradicted(obsReady, "%s never became ready", worker.name)
		c.Contradicted(obsListening, "%s never became ready, so it could not receive", worker.name)
		return false
	}
	c.Observed(obsReady, "selection ready")
	// On this column a selected conversation is what the gateway delivers to,
	// so the same evidence answers both: there is nothing else to wait for.
	c.Observed(obsListening, "the gateway accepts delivery once the conversation is selected")
	return true
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// awaitFirstTwo waits for the recipient's overview to list both of the close
// letters, and answers them as the recipient saw them. Nothing comes back to
// be checked: the case deadline inside Await is the check, and a second answer
// that is always true would only invite a caller to believe it means something.
func awaitFirstTwo(c *Case, worker *codexSession) (alpha, beta peekedMessage) {
	c.Await("the recipient to overview the first two letters", func() bool {
		var seenA, seenB bool
		alpha, seenA = worker.peekedCarrying(batchAlpha)
		beta, seenB = worker.peekedCarrying(batchBeta)
		return seenA && seenB
	})
	return alpha, beta
}

func awaitThird(c *Case, worker *codexSession) peekedMessage {
	var gamma peekedMessage
	c.Await("the recipient to overview the third letter", func() bool {
		var seen bool
		gamma, seen = worker.peekedCarrying(batchGamma)
		return seen
	})
	return gamma
}

// deliveryContaining finds the delivery that named an id. A record the
// scenario cannot read fails the case rather than reading as no delivery.
func deliveryContaining(c *Case, col column, worker *codexSession, id string) groupDelivery {
	var found groupDelivery
	c.Await("a delivery naming "+id, func() bool {
		deliveries, err := col.announcedDeliveries(worker)
		if errors.Is(err, errRecordBehind) {
			// Caught between the delivery and its overview: under a full run
			// that gap is wide enough to land in, and it is progress rather
			// than a contradiction.
			return false
		}
		if err != nil {
			// A membership that cannot be reconciled with what the notice
			// announced is not an absent delivery; the case fails on it.
			c.t.Fatalf("workflow: %v", err)
		}
		for _, delivery := range deliveries {
			if slices.Contains(delivery.Members, id) {
				found = delivery
				return true
			}
		}
		return false
	})
	return found
}

// latestOf orders two overview rows the way the notice must: by time, then
// by id — the product's own tie-break, restated here so the scenario asks
// for exactly what the contract promises.
func latestOf(a, b peekedMessage) (latest, earliest peekedMessage) {
	if b.CreatedAt.After(a.CreatedAt) || b.CreatedAt.Equal(a.CreatedAt) && b.ID > a.ID {
		return b, a
	}
	return a, b
}

// readOf answers whether a read of one id was attempted and whether it
// succeeded.
func readOf(worker *codexSession, id string) (ok, attempted bool) {
	records, err := worker.readRecords()
	if err != nil {
		return false, false
	}
	for _, record := range records {
		if record.Kind == "message" && record.ID == id {
			return record.OK, true
		}
	}
	return false, false
}

// judgeReads decides the two per-message observations from the recipient's
// record: the overview listed the first two and their reads succeeded after
// it, and every letter was read by exactly one call that returned one message.
func judgeReads(c *Case, worker *codexSession, alpha, beta, gamma string) {
	records, err := worker.readRecords()
	if err != nil {
		c.t.Fatalf("workflow: %v", err)
	}
	overviewed := false
	for _, record := range records {
		if record.Kind == "peek" && record.OK && slices.Contains(record.IDs, alpha) && slices.Contains(record.IDs, beta) {
			overviewed = true
			break
		}
	}
	readA, _ := readOf(worker, alpha)
	readB, _ := readOf(worker, beta)
	if overviewed && readA && readB {
		c.Observed(obsPeekConsumedNothing, "both letters were still readable after the overview that listed them")
	} else {
		c.Contradicted(obsPeekConsumedNothing, "overview listed both: %v; read after it: %s %v, %s %v", overviewed, alpha, readA, beta, readB)
	}
	for _, id := range []string{alpha, beta, gamma} {
		calls, returned := 0, 0
		for _, record := range records {
			if record.Kind == "message" && record.ID == id && record.OK {
				calls++
				returned += atoiOr(record.Detail, -1)
			}
		}
		if calls != 1 || returned != 1 {
			c.Contradicted(obsReadOneByOne, "%s was read by %d successful call(s) returning %d message(s)", id, calls, returned)
			return
		}
	}
	c.Observed(obsReadOneByOne, "three letters, three reads, one message each")
}

// judgeNoReplay requires every announced id to have been announced exactly
// once — availability notices included, which is why it looks at every
// delivery and not only at the three letters.
// judgeNoReplay asks the question the way this column can answer it: by the
// ids a delivery named where it names them, and by arithmetic where it does
// not. Both read the record as the session wrote it, never a reconstruction —
// a replay is the one case a reconstruction cannot survive.
func judgeNoReplay(c *Case, col column, worker *codexSession) {
	deliveries, err := worker.groupDeliveries()
	if err != nil {
		c.t.Fatalf("workflow: %v", err)
	}
	if len(deliveries) == 0 {
		c.Contradicted(obsNoReplay, "the recipient recorded no deliveries, so a replay could not have shown")
		return
	}
	again, err := col.replayedAnnouncement(worker)
	if err != nil {
		c.t.Fatalf("workflow: %v", err)
	}
	if again != "" {
		c.Contradicted(obsNoReplay, "%s", again)
		return
	}
	c.Observed(obsNoReplay, fmt.Sprintf("%d deliveries, nothing announced in two of them", len(deliveries)))
}

func announcedTwice(deliveries []groupDelivery) string {
	seen := map[string]bool{}
	for _, delivery := range deliveries {
		for _, id := range delivery.Members {
			if seen[id] {
				return id
			}
			seen[id] = true
		}
	}
	return ""
}

func sorted(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

func atoiOr(text string, fallback int) int {
	n := 0
	if _, err := fmt.Sscanf(text, "%d", &n); err != nil {
		return fallback
	}
	return n
}

// readsOf counts how the reads of these letters came out: how many were
// attempted, and how many of those the product allowed. Counting rather than
// naming, because which letter a group hands over first follows the order of
// their ids, which is not the order they were sent.
func readsOf(worker *codexSession, ids ...string) (attempted, succeeded int) {
	for _, id := range ids {
		ok, tried := readOf(worker, id)
		if !tried {
			continue
		}
		attempted++
		if ok {
			succeeded++
		}
	}
	return attempted, succeeded
}
