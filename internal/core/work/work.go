// Package work is a run's room work as the input of its pane shows it
// (docs/shell.md, "The pane"): the briefs it was given, with their ticks,
// and the reports it waits on. It only reads.
package work

import (
	"fmt"
	"strings"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
)

// Of is the run's work read from the room: the open briefs bound to it and
// what it sent and still waits a report on. What cannot be read is left
// out: the line only informs, and the next read tries again.
func Of(home taskbrief.Home, name, epoch string) string {
	briefs, _, err := taskbrief.List(home, false)
	if err != nil {
		briefs = nil
	}
	return Line(briefs, name, epoch, Awaited(home.Room, name, epoch))
}

// Awaited counts the reports the run waits on, as rewake inbox --awaited's
// header does; 0 when its mail does not read.
func Awaited(room, name, epoch string) int {
	standings, err := mail.Open(room, name, epoch).Standings()
	if err != nil {
		return 0
	}
	coming := 0
	for _, standing := range standings {
		if standing.Coming() {
			coming++
		}
	}
	return coming
}

// Line is the work as shown: each open brief bound to the run, oldest first,
// as `brief 57 3/6` (a brief without items as `brief 57`), then `⧗ 3` for
// the reports it waits on; empty when there is none.
func Line(briefs []taskbrief.Brief, name, epoch string, awaited int) string {
	var parts []string
	for i := len(briefs) - 1; i >= 0; i-- {
		brief := briefs[i]
		if brief.IsClosed() {
			continue
		}
		if _, bound := brief.BindingOf(taskbrief.Author{Name: name, Epoch: epoch}); !bound {
			continue
		}
		part := "brief " + taskbrief.FormatID(brief.ID)
		if items := len(brief.Items()); items > 0 {
			part += fmt.Sprintf(" %d/%d", brief.Done(), items)
		}
		parts = append(parts, part)
	}
	if awaited > 0 {
		parts = append(parts, fmt.Sprintf("⧗ %d", awaited))
	}
	return strings.Join(parts, " · ")
}
