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

// The scenario's negative controls, each a mutation of the product built
// through an overlay. Every control names the observation its breakage must
// take down, and each is verified crosswise: it must go red from its own
// breakage and must not go green from another's. The cross runs under its own
// switch because it multiplies the scenario by the number of pairs.

// The collection window is one product constant. Widened, the third letter
// joins the first two and the group is no longer one of two.
var mutantWindow = mutation{
	name: "window",
	file: "internal/inbox/serve.go",
	edits: []edit{{
		"collectionInterval = 150 * time.Millisecond",
		"collectionInterval = 10 * time.Second",
	}},
}

// The preview is taken from the latest member by one comparison. Reversed,
// the notice previews the earliest.
var mutantPreview = mutation{
	name: "preview",
	file: "internal/harness/notice.go",
	edits: []edit{{
		"member.CreatedAt.After(latest.CreatedAt) || member.CreatedAt.Equal(latest.CreatedAt) && member.ID > latest.ID",
		"member.CreatedAt.Before(latest.CreatedAt) || member.CreatedAt.Equal(latest.CreatedAt) && member.ID < latest.ID",
	}},
}

// An overview consumes nothing because its branch never marks. This makes it
// mark, the way a read does.
var mutantPeekConsumes = mutation{
	name: "peek-consumes",
	file: "internal/cli/inbox.go",
	edits: []edit{{
		"failure = failf(\"could not print the inbox overview: %v\", err)\n" +
			"\t\t\t\treturn nil\n\t\t\t}\n\t\t\treturn nil",
		"failure = failf(\"could not print the inbox overview: %v\", err)\n" +
			"\t\t\t\treturn nil\n\t\t\t}\n" +
			"\t\t\tfor _, message := range messages {\n" +
			"\t\t\t\t_ = inbox.MarkRead(dir, session.Name, epoch, message, !role.Of(session.Role).Silent)\n" +
			"\t\t\t}\n\t\t\treturn nil",
	}},
}

// A delivered message leaves the waiting set — its waiting copy is removed
// once the notice is out — and a pass that still finds it treats it as
// settled by its recorded outcome. Both halves of that guard go here: the copy
// stays, and a delivered outcome no longer counts as settled. What is left is
// exactly the replay the contract forbids, gated by the retry interval.
var mutantReplay = mutation{
	name: "replay",
	file: "internal/inbox/outcome.go",
	edits: []edit{
		{
			"\tcase Delivered, Read:\n\t\twaiting :=",
			"\tcase Delivered:\n\tcase Read:\n\t\twaiting :=",
		},
		{
			"if result, known := s.outcomes[message.ID]; known {\n" +
				"\t\ts.publish(message.ID, result)\n\t\treturn true\n\t}\n" +
				"\tstatus, ok := ReadStatus(s.Dir, s.Name, message.ID)\n" +
				"\tif !ok || status.State == Pending {",
			"if result, known := s.outcomes[message.ID]; known && result.State != Delivered {\n" +
				"\t\ts.publish(message.ID, result)\n\t\treturn true\n\t}\n" +
				"\tstatus, ok := ReadStatus(s.Dir, s.Name, message.ID)\n" +
				"\tif !ok || status.State == Pending || status.State == Delivered {",
		},
	},
}

// batchControl pairs a mutation with the observation it must take down.
type batchControl struct {
	mutant   mutation
	expected string
}

var batchControls = []batchControl{
	{mutantWindow, obsGroupOfTwo},
	{mutantPreview, obsPreviewLatest},
	{mutantPeekConsumes, obsPeekConsumedNothing},
	{mutantReplay, obsNoReplay},
}

func TestAWidenedWindowFails(t *testing.T)    { failedBatchArrival(t, batchControls[0]) }
func TestAnEarliestPreviewFails(t *testing.T) { failedBatchArrival(t, batchControls[1]) }
func TestAConsumingOverviewFails(t *testing.T) {
	failedBatchArrival(t, batchControls[2])
}
func TestAReplayedAnnouncementFails(t *testing.T) { failedBatchArrival(t, batchControls[3]) }

// failedBatchArrival runs the scenario against the control's own mutant and
// requires the observation it names to be the one that breaks.
func failedBatchArrival(t *testing.T, control batchControl) {
	t.Helper()
	enterScenario(t, "batch-arrival-control-"+control.mutant.name)
	runBatchControl(t, "batch-arrival-control-"+control.mutant.name, control.mutant, control.expected, mustBreak)
}

// crossSwitch turns the crosswise check on. It runs every control against
// every other control's mutant and requires each observation to stand — a
// control that also breaks on somebody else's mutation is not observing what
// it claims. It has its own switch because it multiplies the scenario by the
// number of pairs.
const crossSwitch = "REWAKE_WORKFLOW_CROSS"

func TestBatchControlsCrosswise(t *testing.T) {
	if os.Getenv(crossSwitch) == "" {
		t.Skipf("crosswise check skipped: set %s=1 to run every control against every other mutant", crossSwitch)
	}
	enterScenario(t, "batch-arrival-crosswise")
	for _, control := range batchControls {
		for _, other := range batchControls {
			if other.mutant.name == control.mutant.name {
				continue
			}
			// One subtest per pair, because a case carries its own deadline
			// and twelve sharing one exhausted it: the pair running last was
			// blamed for a clock it never used.
			name := control.mutant.name + "-under-" + other.mutant.name
			t.Run(name, func(t *testing.T) {
				runBatchControl(t, "batch-arrival-cross-"+name, other.mutant, control.expected, mustHold)
			})
		}
	}
}

// whatIsExpected says which way a pair must come out. Against its own mutant a
// control must break the observation it names; against another control's
// mutant that same observation must still stand. Both are verdicts about the
// observation, so both are recorded as observations of the case rather than
// left to the caller — a pair that comes out the other way is a finding.
type whatIsExpected int

const (
	mustBreak whatIsExpected = iota
	mustHold
)

func (w whatIsExpected) observation() string {
	if w == mustBreak {
		return "the control broke what it meant to break"
	}
	return "the observation survived another control's breakage"
}

// runBatchControl runs the scenario's sessions against a mutant and records
// whether the named observation came out the way this pair requires.
func runBatchControl(t *testing.T, name string, m mutation, expected string, want whatIsExpected) {
	t.Helper()
	binary := buildMutant(t, m)
	c := Start(t, Spec{
		Name:         name,
		Harness:      "codex",
		Observations: []string{want.observation()},
		Deadline:     90 * time.Second,
	})
	iso := Isolate(t, c, binary)
	worker, sender := startBatchSessions(t, c, iso)
	defer stopSession(t, c, worker)
	defer stopSession(t, c, sender)

	// Anchored on what the judgement needs, not on a pause: all three letters
	// listed in an overview, and both close letters attempted. The deferred
	// member is read at the second delivery in a healthy run, at the first
	// under the widened window, and at the replayed one under the replay — and
	// that last case is why the third letter belongs in the anchor. A replay is
	// gated by the server's retry interval, two seconds, while the third letter
	// is sent after three, so under the replay mutant the reads are all done
	// before the third letter exists; without it in the anchor the grouping
	// question was judged on a record that could not yet answer it, and said
	// so.
	if !waitFor(c, batchWindow, func() bool {
		alpha, seenA := worker.peekedCarrying(batchAlpha)
		beta, seenB := worker.peekedCarrying(batchBeta)
		_, seenC := worker.peekedCarrying(batchGamma)
		if !seenA || !seenB || !seenC {
			return false
		}
		_, triedA := readOf(worker, alpha.ID)
		_, triedB := readOf(worker, beta.ID)
		return triedA && triedB
	}) {
		c.Contradicted(want.observation(), "the three letters were not all listed and read, so nothing could be judged")
		return
	}
	// No settling pause: every breakage this asks about is already in the
	// recipient's record by the time the anchor is reached. The replay shows
	// as the redelivery whose read the anchor waited for, and the other three
	// are properties of the delivery that carried those letters.
	c.Note("watching " + expected + " under " + m.name)
	broke, why, err := batchControlOutcome(expected, worker)
	switch {
	case err != nil:
		// Not being able to look is not the same as having looked: a record
		// the check cannot read fails the pair rather than reading as an
		// observation that held.
		c.Contradicted(want.observation(), "could not judge %s under %s: %v", expected, m.name, err)
	case want == mustBreak && broke:
		c.Observed(want.observation(), why)
	case want == mustBreak:
		c.Contradicted(want.observation(), "%s held anyway: %s", expected, why)
	case broke:
		c.Contradicted(want.observation(), "%s broke under %s, whose breakage it does not cover: %s", expected, m.name, why)
	default:
		c.Observed(want.observation(), fmt.Sprintf("%s held under %s: %s", expected, m.name, why))
	}
}

// batchWindow bounds the wait for the anchor. The healthy run reaches it a
// little after laterSendDelay; the widened window adds its own ten seconds.
const batchWindow = 45 * time.Second

// batchControlOutcome asks for the full shape of each breakage, and answers in
// three values rather than two: broken, not broken, or not judgeable. A branch
// that folded the third into the second would call a record it could not read
// an observation that held.
func batchControlOutcome(expected string, worker *codexSession) (bool, string, error) {
	deliveries, err := worker.groupDeliveries()
	if err != nil {
		return false, "", err
	}
	// No deliveries at all is the absence of evidence, not evidence of
	// absence: the file is missing or empty, and every branch below would
	// otherwise answer that nothing was announced twice and no delivery named
	// all three — the collapse the three-valued answer exists to prevent, and
	// exactly what a renamed switch would produce.
	if len(deliveries) == 0 {
		return false, "", errors.New("the recipient recorded no deliveries, so nothing can be judged")
	}
	alpha, seenA := worker.peekedCarrying(batchAlpha)
	beta, seenB := worker.peekedCarrying(batchBeta)
	gamma, seenC := worker.peekedCarrying(batchGamma)
	if !seenA || !seenB {
		return false, "", errors.New("the recipient never listed the first two letters")
	}
	switch expected {
	case obsGroupOfTwo:
		if !seenC {
			return false, "", errors.New("the recipient never listed the third letter, so grouping cannot be judged")
		}
		for _, delivery := range deliveries {
			if slices.Contains(delivery.Members, alpha.ID) && slices.Contains(delivery.Members, beta.ID) && slices.Contains(delivery.Members, gamma.ID) {
				return true, "one delivery named all three letters: " + strings.Join(delivery.Members, ","), nil
			}
		}
		return false, "no delivery named all three letters", nil
	case obsPreviewLatest:
		latest, earliest := latestOf(alpha, beta)
		for _, delivery := range deliveries {
			if slices.Contains(delivery.Members, alpha.ID) && slices.Contains(delivery.Members, beta.ID) &&
				strings.Contains(delivery.Notice, earliest.Preview) && !strings.Contains(delivery.Notice, latest.Preview) {
				return true, "the group's notice previews the earliest member, " + earliest.Preview, nil
			}
		}
		return false, "no delivery of the two close letters previewed the earliest of them", nil
	case obsPeekConsumedNothing:
		records, err := worker.readRecords()
		if err != nil {
			return false, "", err
		}
		if len(records) == 0 {
			return false, "", errors.New("the recipient recorded no inbox calls, so nothing can be judged")
		}
		listed := false
		for _, record := range records {
			if record.Kind == "peek" && record.OK && slices.Contains(record.IDs, alpha.ID) && slices.Contains(record.IDs, beta.ID) {
				listed = true
			}
		}
		// The overview listed both letters and the reads that followed were
		// refused: the overview consumed what it only meant to show. Which of
		// the two is read first follows the notice's order, so both are asked.
		okA, triedA := readOf(worker, alpha.ID)
		okB, triedB := readOf(worker, beta.ID)
		if listed && triedA && triedB && !okA && !okB {
			return true, "the overview listed both letters and the reads that followed were refused", nil
		}
		return false, "the letters stayed readable after the overview", nil
	case obsNoReplay:
		if again := announcedTwice(deliveries); again != "" {
			return true, again + " was announced twice", nil
		}
		return false, "no id was announced twice", nil
	}
	return false, "", errors.New("no such observation: " + expected)
}
