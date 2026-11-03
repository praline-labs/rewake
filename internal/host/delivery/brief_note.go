package delivery

import (
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/reportbrief"
	"github.com/praline-labs/rewake/internal/core/taskbrief"
	"github.com/praline-labs/rewake/internal/host/launch"
)

// noteBriefs gives each report on a brief's task the note its notice shows,
// "brief 96 7/8", read from the brief now (docs/delivery.md#the-notice). The
// note lives on the copy handed to the adapter and is never written down; a
// brief or a task that cannot be read leaves the notice as it was.
func (s *Server) noteBriefs(ready []mail.Message) {
	var home *taskbrief.Home
	for index := range ready {
		if !mail.IsReport(ready[index]) || len(ready[index].InReplyTo) == 0 {
			continue
		}
		if home == nil {
			found := launch.QuietBriefHome(s.Dir)
			home = &found
		}
		ready[index] = ready[index].WithBriefNote(reportbrief.Notes(reportbrief.Of(s.Dir, *home, ready[index])))
	}
}
