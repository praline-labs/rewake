package workflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestQuestionAnswered is a blocking question end to end: one running session
// sends `rewake send <name> "..." --question` and waits; the other reads the
// question and ends its turn; the waiting command prints that end as the
// answer. A question is a task whose sender waits, so the answer must not also
// arrive as the ordinary report a task gets: the asker is handed it by the
// command and is announced nothing.
//
// The asker runs its own command, as its session would: a blocking send is the
// asker's, and a test process calling it would prove nothing about the run's
// waiting mark or the reservation of the answer.
func TestQuestionAnswered(t *testing.T) {
	runInColumns(t, "question", func(t *testing.T, col column) {
		binary := enterScenario(t, "question")
		c := Start(t, Spec{
			Name:         "question",
			Harness:      col.harness,
			Observations: questionObservations,
			Deadline:     90 * time.Second,
		})
		judge(c, playQuestion(t, c, Isolate(t, c, binary), col))
	})
}

const (
	obsQuestionAnswered = "the asker's rewake send --question exits 0 and prints the answer from the session it asked, made of what that session read"
	obsQuestionOnce     = "the session asked is shown the question once, in one delivery, and reads it"
	obsQuestionAlone    = "the answer reaches the asker only through the command: nothing is delivered to it, it reads nothing from the session asked, and its inbox holds nothing unread"
	questionText        = "question-probe: which port does the staging api listen on"
)

var questionObservations = []string{obsQuestionAnswered, obsQuestionOnce, obsQuestionAlone}

func playQuestion(t *testing.T, c *Case, iso *Isolation, col column) []telemetryFinding {
	t.Helper()
	worker := startHarnessSession(t, c, iso, col.harness, "worker", "--general", shimInboxJSON+"=1")
	defer stopSession(t, c, worker)
	asks := newRequests(iso, "asker")
	asker := startHarnessSession(t, c, iso, col.harness, "asker", "--main", shimInboxJSON+"=1", asks.env())
	defer stopSession(t, c, asker)

	unjudged := func(detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range questionObservations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}
	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	if err := sessionsUp(c, worker, asker); err != nil {
		return unjudged(err.Error())
	}

	code, out, ok := asks.ask(c, "send", worker.name, questionText, "--question", "--wait", "15")
	if !ok {
		return unjudged("the asker's command did not come back: " + out)
	}

	var findings []telemetryFinding
	switch {
	case code != 0:
		findings = append(findings, finding(obsQuestionAnswered, false, "exit %d: %q", code, out))
	case !strings.Contains(out, "answer from "+worker.name+":") || !strings.Contains(out, "question-probe"):
		findings = append(findings, finding(obsQuestionAnswered, false, "exit 0, but the output holds no answer made of what %s read: %q", worker.name, out))
	default:
		findings = append(findings, finding(obsQuestionAnswered, true, "exit 0: %s", strings.ReplaceAll(strings.TrimSpace(out), "\n", " / ")))
	}

	asked, read := messageCarrying(worker, "question-probe")
	deliveries := slices.DeleteFunc(worker.deliveredIDs(), func(id string) bool { return id != asked.ID })
	switch {
	case !read || asked.Kind != "question":
		findings = append(findings, finding(obsQuestionOnce, false, "the session asked read %+v as the question", asked))
	case len(deliveries) != 1:
		findings = append(findings, finding(obsQuestionOnce, false, "the question %s was named by %d deliveries", asked.ID, len(deliveries)))
	default:
		findings = append(findings, finding(obsQuestionOnce, true, "question %s delivered and read once", asked.ID))
	}

	// The worker's heads-up that it is up is a notify the asker may have read
	// before it asked; any other kind, or any delivery of one that was not read
	// as such, is what the question must not leave behind.
	var strays []string
	headsUps := map[string]bool{}
	for _, message := range readMessages(asker) {
		if message.Kind == "notify" {
			headsUps[message.ID] = true
		} else {
			strays = append(strays, message.Kind+" "+message.ID)
		}
	}
	for _, id := range asker.deliveredIDs() {
		if !headsUps[id] {
			strays = append(strays, "delivered "+id)
		}
	}

	// The asker's inbox is looked at through its own rewake: a report or a
	// notice that was written with the answer is unread there now.
	codePeek, peek, peeked := asks.ask(c, "inbox", "--peek")
	switch {
	case !peeked || codePeek != 0:
		findings = append(findings, finding(obsQuestionAlone, false, "the asker's inbox did not come back: exit %d, %q", codePeek, peek))
	case len(strays) != 0:
		findings = append(findings, finding(obsQuestionAlone, false, "the asker was delivered or read more than the worker's heads-up: %v", strays))
	case strings.Contains(peek, worker.name):
		findings = append(findings, finding(obsQuestionAlone, false, "the asker's inbox holds mail from %s: %q", worker.name, peek))
	default:
		findings = append(findings, finding(obsQuestionAlone, true, "nothing delivered to the asker, nothing unread: %s", firstLine(strings.TrimSpace(peek))))
	}
	return findings
}
