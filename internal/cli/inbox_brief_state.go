package cli

import (
	"github.com/praline-labs/rewake/internal/core/reportbrief"
)

// withBriefStates reads, for each report on a task given with --brief, the
// state its brief has now: what is still unticked and what was said in it
// since the task (docs/briefs.md#what-main-reads). Nothing is copied into
// the report.
func withBriefStates(dir string, views []messageView) []messageView {
	home := quietHomeOf(dir)
	for index := range views {
		views[index].BriefState = reportbrief.Of(dir, home, views[index].Message)
	}
	return views
}

// briefStateLines print the blocks under a report's text.
func briefStateLines(blocks []reportbrief.Block) []string {
	var lines []string
	for _, block := range blocks {
		lines = append(lines, "")
		lines = append(lines, block.Lines()...)
	}
	return lines
}
