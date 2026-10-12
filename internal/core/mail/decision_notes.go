package mail

import (
	"fmt"
	"sort"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// The notes a decision owes are frozen when it is written (Q11 of
// docs/v2/stage3-decision-build.md): the recipient's run and the room's main
// run as they are then, each note whole with its id. A later role change or a
// later main retargets none of them, and whoever installs them reads them
// from the decision alone. The run that decided, verified, is told nothing:
// it made the decision. The person's path owes both distinct runs.

// noteRuns are the runs a decision on subject made by by owes a note: the
// recipient's run unless it decided, and the room's live main unless it is
// that run or the one that decided. A main inventory that cannot be read
// refuses the decision rather than leave out a note it may owe; a room found
// positively without a live main owes none.
func noteRuns(dir string, subject Subject, by DecidedBy) (recipient bool, main registry.Session, owesMain bool, err error) {
	deciding := ""
	if by.Verified {
		deciding = by.Run
	}
	recipient = subject.RecipientRun != deciding
	found, err := registry.Inventory(dir)
	if err != nil {
		return false, registry.Session{}, false, refuseDecision(DecisionUnreadable, "", "the room's main cannot be found, so the notes a decision owes are unknown: %v", err)
	}
	var mains []registry.Session
	for _, record := range found {
		if role.Of(record.Session.Role).ID != role.Main.ID {
			continue
		}
		switch record.Run {
		case proc.IdentityAlive:
			mains = append(mains, record.Session)
		case proc.IdentityUnknown:
			return false, registry.Session{}, false, refuseDecision(DecisionUnreadable, registry.RecordPath(dir, record.Session.Name), "whether the room's main %s runs cannot be told, so the notes a decision owes are unknown", record.Session.Name)
		}
	}
	if len(mains) == 0 {
		return recipient, registry.Session{}, false, nil
	}
	// The oldest, as the stop's own note to main takes it (roomMainOf).
	sort.SliceStable(mains, func(i, j int) bool { return mains[i].StartedAt.Before(mains[j].StartedAt) })
	main = mains[0]
	run := main.Epoch()
	return recipient, main, run != subject.RecipientRun && run != deciding, nil
}

// decisionNotes are the notes of decision d, frozen.
func decisionNotes(dir string, d Decision) ([]DecisionNote, error) {
	s := d.Subject
	recipient, main, owesMain, err := noteRuns(dir, s, d.By)
	if err != nil {
		return nil, err
	}
	text := noteText(d)
	note := func(told, to, run string) DecisionNote {
		return DecisionNote{Told: told, Note: Message{
			ID: NewID(), Kind: Note, From: s.Sender, FromEpoch: s.SenderRun, To: to, ToEpoch: run, Text: text, CreatedAt: d.At,
		}}
	}
	var notes []DecisionNote
	if recipient {
		notes = append(notes, note(ToldRecipient, s.Recipient, s.RecipientRun))
	}
	if owesMain {
		notes = append(notes, note(ToldMain, main.Name, main.Epoch()))
	}
	return notes, nil
}

// noteText says what was decided, by whom, and that nothing was proven.
func noteText(d Decision) string {
	s := d.Subject
	who := "the person, unverified"
	if d.By.Verified {
		who = fmt.Sprintf("%s (run %s, verified)", d.By.Name, d.By.Run)
	}
	choice := "arrived"
	then := ""
	if d.Choice == ChoiceNotArrived {
		choice = "did not arrive"
		then = fmt.Sprintf(" It is sent again under decision %s.", d.ID)
	}
	return fmt.Sprintf("Report %s of %s to %s (run %s) was decided as %s by %s, at %s; whether it landed is not proven.%s",
		s.Report, s.Sender, s.Recipient, s.RecipientRun, choice, who, d.At.UTC().Format(time.RFC3339), then)
}
