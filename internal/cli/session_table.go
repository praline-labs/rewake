package cli

import (
	"cmp"
	"fmt"
	"math"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/praline-labs/rewake/internal/core/session"
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
	// The column is there only when some session has a type, so a room
	// without typed workers keeps the table it had.
	typed := false
	for _, session := range sessions {
		typed = typed || session.Type != ""
	}
	header := []string{"Session", "Role"}
	if typed {
		header = append(header, "Type")
	}
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
		if typed {
			row = append(row, cmp.Or(session.Type, "-"))
		}
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
	lines = append(lines, strings.TrimSuffix(buffer.String(), "\n"))
	return lines
}

func stateCells(snapshot *session.Snapshot) []string {
	if snapshot == nil {
		return []string{"unknown", "unknown", "unknown", "unknown", "unknown"}
	}
	model := snapshot.Model
	if snapshot.ModelName != nil {
		model = snapshot.ModelName
	}
	activity := activityText(snapshot)
	if !snapshot.Fresh && !strings.Contains(activity, "stale") {
		activity += " (unavailable/stale)"
	}
	setting := func(value *string, source string) string {
		if value == nil {
			return "unknown"
		}
		text := strconv.Quote(*value)
		if source != "" {
			text += " (" + source + ")"
		}
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
	return []string{activity, setting(model, ""), setting(snapshot.Effort, snapshot.EffortSource), context, count}
}
