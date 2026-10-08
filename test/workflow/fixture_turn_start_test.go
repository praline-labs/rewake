package workflow

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"
)

// The fixture program's timed turn starts (docs/v2/stage3-fixture.md): the
// program times each turn's start itself and sends that time before the turn
// does anything; the endpoint records it in the mailbox, where a pending mark
// of an earlier turn whose end was lost finds its turn over. The record is
// read where the mailbox keeps it, through a launched fixture, over two turns.

const (
	obsTurnStartRecorded = "each turn of the fixture program records its own timed start in the mailbox, a later one for the later turn"
	turnStartFirst       = "fixture-turn-start: the first task"
	turnStartSecond      = "fixture-turn-start: the second task"
)

func TestFixtureTurnStart(t *testing.T) {
	runParallel(t)
	col := fixtureColumn
	t.Run(col.harness, func(t *testing.T) {
		binary := enterScenario(t, "fixture-turn-start")
		c := Start(t, Spec{
			Name:         "fixture-turn-start",
			Harness:      col.harness,
			Observations: []string{obsTurnStartRecorded},
			Deadline:     90 * time.Second,
		})
		for _, finding := range playTurnStart(t, c, Isolate(t, c, binary)) {
			if finding.held {
				c.Observed(finding.observation, finding.detail)
			} else {
				c.Contradicted(finding.observation, "%s", finding.detail)
			}
		}
	})
}

func playTurnStart(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	harness := fixtureColumn.harness
	worker := startHarnessSession(t, c, iso, harness, "worker", "--general", shimInboxJSON+"=1")
	defer stopSession(t, c, worker)
	leadAsks := newRequests(iso, "lead")
	lead := startHarnessSession(t, c, iso, harness, "lead", "--main", shimInboxJSON+"=1", leadAsks.env())
	defer stopSession(t, c, lead)

	var starts []int64
	for _, text := range []string{turnStartFirst, turnStartSecond} {
		code, sent, _ := leadAsks.ask(c, "send", worker.name, text)
		answered := waitFor(c, 30*time.Second, func() bool {
			task, ok := messageCarrying(worker, text)
			return ok && slices.ContainsFunc(readMessages(lead), func(message reportView) bool {
				return message.From == worker.name && slices.Contains(message.InReplyTo, task.ID)
			})
		})
		if !answered {
			return []telemetryFinding{{observation: obsTurnStartRecorded, detail: fmt.Sprintf("the worker never answered %q (send: exit %d, %s); starts so far %v", text, code, firstLine(sent), starts)}}
		}
		starts = append(starts, latestTurnStart(iso, worker.name))
	}
	held := starts[0] > 0 && starts[1] > starts[0]
	return []telemetryFinding{toolFinding(obsTurnStartRecorded, held, "the latest start after each turn: %v", starts)}
}

// latestTurnStart is the latest turn start a session's mailbox records, 0 when
// none: the largest reading under its runs' .turn-starts.
func latestTurnStart(iso *Isolation, name string) int64 {
	readings, _ := filepath.Glob(filepath.Join(iso.StateDir, "rooms", "default", "inbox", name, "awaiting", "*", ".turn-starts", "*"))
	var latest int64
	for _, reading := range readings {
		if value, err := strconv.ParseInt(filepath.Base(reading), 10, 64); err == nil && value > latest {
			latest = value
		}
	}
	return latest
}
