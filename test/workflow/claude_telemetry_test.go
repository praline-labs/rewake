package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Latency budgets for the telemetry commands, in milliseconds, measured on the
// development machine in September 2026 with a cold rewake process each time
// (docs/testing.md has the figures). The hooks sit in front of every prompt;
// the tap sits in front of the person's status line. A budget several times
// the measurement catches a regression — a lock, a wait, a heavy start — and
// not a busy machine.
const (
	hookBudgetMedian = 20.0
	hookBudgetP95    = 50.0
	tapBudgetMedian  = 20.0
)

// ownerStatusCommand is the person's own status line in the case: it prints
// the model id it finds on stdin, so its output shows both that it ran and
// that what the harness handed the tap reached it.
const ownerStatusCommand = `sed -n 's/.*"id":"\([^"]*\)".*/owner \1/p'`

// TestClaudeTelemetry is the collector end to end on the Claude Code column,
// seen from where it is used: a main and a worker. The worker's hooks and
// status line, run as the harness runs them, reach the main's `rewake list` as
// model, effort, context, compactions and activity; the main's header on a
// message from the worker shows the same; a compaction on the worker is
// announced to the main; the person's status line is still what is shown;
// and the commands stay cheap.
//
// Only this column: the Codex column's telemetry comes from its server and is
// exercised by every scenario that waits on it.
//
// What it does not prove: that the real harness calls these commands when it
// says it does. That is recorded from a live session in docs/research.md; the
// fixture plays the payloads seen there.
func TestClaudeTelemetry(t *testing.T) {
	binary := enterScenario(t, "claude-telemetry")
	c := Start(t, Spec{
		Name:         "claude-telemetry",
		Harness:      claudeColumn.harness,
		Observations: telemetryObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playClaudeTelemetry(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsTelemetryValues  = "the worker's model, effort and context reach the main's rewake list"
	obsCompactionCount  = "a compaction on the worker is counted from its hooks"
	obsIdleAfterTurn    = "the worker reads idle after its turn"
	obsConversation     = "the worker's conversation is followed"
	obsHeader           = "the main's header on the worker's message shows its state"
	obsCompactionNotice = "the worker's compaction is announced to the main"
	obsOwnerStatusShown = "the person's status line is what is shown"
	obsHookBudget       = "a telemetry hook stays within its budget"
	obsTurnStartBudget  = "the hook that records a turn start stays within the same budget"
	obsTapBudget        = "the tap adds little to the person's status line"
)

var telemetryObservations = []string{
	obsTelemetryValues, obsCompactionCount, obsIdleAfterTurn, obsConversation,
	obsHeader, obsCompactionNotice, obsOwnerStatusShown, obsHookBudget, obsTurnStartBudget, obsTapBudget,
}

// telemetryFinding is one observation's answer. judged is false when the
// sessions' records could not be read at all, which is never "held".
type telemetryFinding struct {
	observation string
	held        bool
	judged      bool
	detail      string
}

// playClaudeTelemetry runs a main and a worker, the worker through the
// telemetry script, and answers every observation from the main's side.
func playClaudeTelemetry(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	settings, _ := json.Marshal(map[string]any{"statusLine": map[string]string{"type": "command", "command": ownerStatusCommand}})
	if err := os.MkdirAll(filepath.Join(iso.Home, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iso.Home, ".claude", "settings.json"), settings, 0o600); err != nil {
		t.Fatal(err)
	}
	unjudged := func(detail string) []telemetryFinding {
		var out []telemetryFinding
		for _, observation := range telemetryObservations {
			out = append(out, telemetryFinding{observation: observation, detail: detail})
		}
		return out
	}

	// The main first: a compaction is announced only when it completes after
	// the main started, and the worker compacts almost at once.
	lead := startHarnessSession(t, c, iso, claudeColumn.harness, "lead", "--main", shimReportListing+"=1")
	defer stopSession(t, c, lead)
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(lead.ready); return err == nil }) {
		return unjudged("the main never started listening")
	}
	evidence := filepath.Join(iso.Home, "telemetry.json")
	worker := startHarnessSession(t, c, iso, claudeColumn.harness, "worker", "--general",
		shimTelemetryFile+"="+evidence, shimOwnerStatus+"="+ownerStatusCommand, shimTelemetryNotify+"="+lead.name)
	defer stopSession(t, c, worker)

	result, err := readTelemetryResult(c, evidence)
	if err != nil {
		return unjudged(err.Error())
	}
	// Anchored on what no control touches: the last status and the end of the
	// turn. The script has finished by the time its evidence exists, so what
	// the listing says once these arrived is what the collector made of it.
	row, ok := awaitTelemetry(c, lead, worker.name, func(row telemetryRow) bool {
		return row.Activity == "idle" && row.Used != nil && *row.Used == 50000
	})
	listingFailed := ""
	if timeouts, err := os.ReadFile(lead.state + ".timeouts"); err == nil && len(timeouts) > 0 {
		listingFailed = "a listing timed out: " + strings.TrimSpace(string(timeouts))
		c.Note(listingFailed)
	}
	if !ok && listingFailed == "" {
		listingFailed = fmt.Sprintf("the main's listing never showed the end of the worker's turn: %+v", row)
	}
	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	var out []telemetryFinding
	if listingFailed != "" {
		for _, observation := range []string{obsTelemetryValues, obsCompactionCount, obsIdleAfterTurn, obsConversation} {
			out = append(out, telemetryFinding{observation: observation, detail: listingFailed})
		}
	} else {
		values := row.Model != nil && *row.Model == "model-telemetry" && row.Effort != nil && *row.Effort == "high" &&
			row.Window != nil && *row.Window == 200000 && row.Percent != nil && *row.Percent == 25
		out = append(out, finding(obsTelemetryValues, values, "model %s, effort %s, %d of %s (%s%%)",
			show(row.Model), show(row.Effort), *row.Used, show(row.Window), show(row.Percent)))
		counted := row.Compactions != nil && *row.Compactions == 1 && row.Coverage == "observed" && row.Compacting != nil && !*row.Compacting
		out = append(out, finding(obsCompactionCount, counted, "completed %s, coverage %q, in progress %s",
			show(row.Compactions), row.Coverage, show(row.Compacting)))
		out = append(out, finding(obsIdleAfterTurn, true, "%s", row.Activity))
		out = append(out, finding(obsConversation, row.Thread == telemetryConversation, "thread %q", row.Thread))
	}

	// The main reads its mail on every delivery and the fixture appends what
	// it read. Anchored on the worker's notify; the compaction notice, sent by
	// the main's own wrapper once a second, gets a few seconds beyond it.
	read := func() string { return lead.mailboxRead() }
	if !waitFor(c, 20*time.Second, func() bool { return strings.Contains(read(), "telemetry played") }) {
		out = append(out, telemetryFinding{observation: obsHeader, detail: "the worker's notify never reached the main: " + read()},
			telemetryFinding{observation: obsCompactionNotice, detail: "the main's mail could not be judged"})
	} else {
		notice := "Rewake: context compacted (compaction 1)."
		waitFor(c, 5*time.Second, func() bool { return strings.Contains(read(), notice) })
		header := worker.name + ": idle | context 25% used / 200K | compactions 1"
		mail := read()
		out = append(out, finding(obsHeader, strings.Contains(mail, header), "looked for %q in what the main read", header))
		out = append(out, finding(obsCompactionNotice, strings.Contains(mail, notice), "looked for %q in what the main read", notice))
	}

	shown := len(result.StatusOutputs) == 5 && result.Error == ""
	for _, output := range result.StatusOutputs {
		shown = shown && output == "owner model-telemetry\n"
	}
	out = append(out, finding(obsOwnerStatusShown, shown, "outputs %q, error %q", result.StatusOutputs, result.Error))

	hookMedian, hookP95 := quantile(result.Hook, 0.5), quantile(result.Hook, 0.95)
	out = append(out, finding(obsHookBudget, len(result.Hook) == latencyRuns && hookMedian <= hookBudgetMedian && hookP95 <= hookBudgetP95,
		"hook median %.1f ms, p95 %.1f ms over %d runs; budget %.0f / %.0f", hookMedian, hookP95, len(result.Hook), hookBudgetMedian, hookBudgetP95))
	turnMedian, turnP95 := quantile(result.TurnStart, 0.5), quantile(result.TurnStart, 0.95)
	out = append(out, finding(obsTurnStartBudget, len(result.TurnStart) == latencyRuns && turnMedian <= hookBudgetMedian && turnP95 <= hookBudgetP95,
		"UserPromptSubmit hook median %.1f ms, p95 %.1f ms over %d runs; budget %.0f / %.0f", turnMedian, turnP95, len(result.TurnStart), hookBudgetMedian, hookBudgetP95))
	var added []float64
	for index := range result.Tap {
		added = append(added, result.Tap[index]-result.Owner[index])
	}
	out = append(out, finding(obsTapBudget, len(added) == latencyRuns && quantile(added, 0.5) <= tapBudgetMedian,
		"tap median %.1f ms, p95 %.1f ms; the person's line alone median %.1f ms; added median %.1f ms, p95 %.1f ms; budget %.0f",
		quantile(result.Tap, 0.5), quantile(result.Tap, 0.95), quantile(result.Owner, 0.5), quantile(added, 0.5), quantile(added, 0.95), tapBudgetMedian))
	return out
}

// show prints an optional value, or "unknown".
func show[T any](value *T) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprint(*value)
}

// telemetryRow is one session's telemetry as `rewake list --json` prints it.
type telemetryRow struct {
	Activity    string  `json:"activity"`
	Thread      string  `json:"primaryThread"`
	Model       *string `json:"configuredModel"`
	Effort      *string `json:"configuredReasoningEffort"`
	Used        *int64  `json:"contextUsedTokens"`
	Window      *int64  `json:"contextWindowTokens"`
	Percent     *int    `json:"contextFilledPercent"`
	Compactions *uint64 `json:"completedCompactions"`
	Compacting  *bool   `json:"compactionInProgress"`
	Coverage    string  `json:"compactionCoverage"`
}

// awaitTelemetry reads a session's listing until the named row satisfies the
// predicate or the case runs out of time.
func awaitTelemetry(c *Case, session *codexSession, name string, ready func(telemetryRow) bool) (telemetryRow, bool) {
	var last telemetryRow
	c.Note("waiting for the telemetry to settle")
	for !c.Expired() {
		raw, err := os.ReadFile(session.state)
		var listed struct {
			Sessions []struct {
				Name      string       `json:"name"`
				Telemetry telemetryRow `json:"telemetry"`
			} `json:"sessions"`
		}
		if err == nil && json.Unmarshal(raw, &listed) == nil {
			for _, entry := range listed.Sessions {
				if entry.Name == name {
					last = entry.Telemetry
					if ready(last) {
						return last, true
					}
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return last, false
}

func readTelemetryResult(c *Case, path string) (telemetryResult, error) {
	var result telemetryResult
	for !c.Expired() {
		raw, err := os.ReadFile(path)
		if err == nil {
			if err := json.Unmarshal(raw, &result); err != nil {
				return result, fmt.Errorf("unreadable evidence: %w", err)
			}
			return result, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return result, fmt.Errorf("the session never wrote %s", strings.TrimPrefix(path, "/"))
}
