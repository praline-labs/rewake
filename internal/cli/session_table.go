package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

func sessionTable(room string, sessions []sessionView, visible bool) []string {
	commonDirectory, commonHarness := true, true
	for _, session := range sessions[1:] {
		commonDirectory = commonDirectory && session.CWD == sessions[0].CWD
		commonHarness = commonHarness && session.Harness == sessions[0].Harness
	}
	lines := []string{"Room: " + room}
	if commonDirectory {
		lines = append(lines, "Directory: "+strconv.Quote(sessions[0].CWD))
	}
	if commonHarness {
		lines = append(lines, "Harness: "+strconv.Quote(sessions[0].Harness))
	}
	header := []string{"Session", "Role"}
	if !commonHarness {
		header = append(header, "Harness")
	}
	if visible {
		header = append(header, "Status", "Model", "Effort", "Context", "Compactions")
	}
	header = append(header, "Age")
	if !commonDirectory {
		header = append(header, "Directory")
	}
	var buffer strings.Builder
	writer := tabwriter.NewWriter(&buffer, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(writer, strings.Join(header, "\t"))
	for _, session := range sessions {
		row := []string{session.Name, session.Role}
		if !commonHarness {
			row = append(row, strconv.Quote(session.Harness))
		}
		if visible {
			row = append(row, stateCells(session.Telemetry)...)
		}
		row = append(row, age(session.Age()))
		if !commonDirectory {
			row = append(row, strconv.Quote(session.CWD))
		}
		_, _ = fmt.Fprintln(writer, strings.Join(row, "\t"))
	}
	_ = writer.Flush()
	return append(lines, strings.TrimSuffix(buffer.String(), "\n"))
}

func stateCells(snapshot *sessionstate.Snapshot) []string {
	if snapshot == nil {
		return []string{"unknown", "unknown", "unknown", "unknown", "unknown"}
	}
	activity := activityText(snapshot)
	if !snapshot.Fresh && !strings.Contains(activity, "stale") {
		activity += " (unavailable/stale)"
	}
	setting := func(value *string) string {
		if value == nil {
			return "unknown"
		}
		text := strconv.Quote(*value)
		if !snapshot.Fresh || !snapshot.SettingsFresh {
			text += " (stale)"
		}
		return text
	}
	percent, window, count := "unknown", "unknown", "unknown"
	if snapshot.FilledPercent != nil {
		percent = strconv.Itoa(*snapshot.FilledPercent) + "%"
	}
	if snapshot.ContextWindow != nil {
		window = fmt.Sprintf("%.0fK", math.Round(float64(*snapshot.ContextWindow)/1000))
	}
	context := percent + " / " + window
	if snapshot.ContextAt != nil && (!snapshot.Fresh || !snapshot.ContextFresh) {
		context += " (stale)"
	}
	if snapshot.Compactions != nil {
		count = strconv.FormatUint(*snapshot.Compactions, 10)
	}
	if snapshot.Coverage == "partial" {
		count += " (partial)"
	}
	return []string{activity, setting(snapshot.Model), setting(snapshot.Effort), context, count}
}
