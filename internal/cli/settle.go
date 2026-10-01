package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// settleGroup is the word main or the person gives on a report rewake cannot
// tell was delivered (docs/turn-end-recovery.md#the-stop-and-rewake-settle).
// It is a shell command, not on the mail tool's surface, and a session that
// is not main is refused: a worker settling its own stop would decide the
// evidence it is stopped on.
func settleGroup() Group {
	return Group{
		Title:   "SETTLE A STOPPED MAILBOX",
		Summary: "Main or the person, from a shell: the answer to a mailbox rewake stopped.",
		Commands: []*Command{{
			Name:           "settle",
			Args:           "<name> <report>",
			MaxPositionals: 2,
			Summary:        "Record whether a report an earlier build may have written reached its recipient, and let the stopped mailbox go on.",
			Options: []Option{
				{Flag: "--delivered", Summary: "It arrived: what it answered counts as answered."},
				{Flag: "--undelivered", Summary: "It did not: it goes now, to its recipient's run or that run's successor."},
				jsonOption,
			},
			Examples: []string{
				"rewake settle api 1790794720612927445-9ad0508099e2 --delivered",
				"rewake settle api 1790794720612927445-9ad0508099e2 --undelivered",
			},
			Next: []string{"rewake list"},
			Notes: []string{
				"A mailbox stops when a report of an earlier build's turn end can be neither found nor proven absent: it reads, reports and clears nothing, letters still arrive, and main is told once, with the lines to run. Ask the recipient whether it has that report, then settle it.",
				"One line per report: each is one recipient run and what it answers. While other reports stay unknown, the mailbox stays stopped and the answer lists them.",
				"The decision is written before any effect and kept for good: the same words again answer it, the opposite words are refused naming it, and a report no journal names, or one rewake can tell, is refused.",
				"Only main or the person may settle: run from a session's shell, it is refused unless that session is the room's main.",
				"Exit 0 recorded, or recorded before; 1 the mailbox is busy, or the barrier failed after the decision was recorded (the next turn end or settle looks again); 2 a wrong call — neither or both of --delivered and --undelivered, a report the journal does not allow settling, a session other than main settling.",
			},
			Handler: handleSettle,
		}},
	}
}

// settleModel is what rewake settle answers.
type settleModel struct {
	Session   string   `json:"session"`
	Report    string   `json:"report"`
	Delivered bool     `json:"delivered"`
	Recorded  bool     `json:"recorded"`
	Stopped   bool     `json:"stopped"`
	Remaining []string `json:"remaining,omitempty"`
}

func handleSettle(ctx *Context, call Call) error {
	usage := func(format string, args ...any) error {
		return &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	if len(call.Positionals) < 2 {
		return usage("rewake settle needs the session and the report, as the note about the stop names them.")
	}
	name, report := strings.TrimSpace(call.Positionals[0]), strings.TrimSpace(call.Positionals[1])
	if !state.ValidName(name) {
		return usage("%q is not a session address; take the one the note about the stop names.", name)
	}
	delivered, undelivered := call.Switch("delivered"), call.Switch("undelivered")
	if delivered == undelivered {
		return usage("say what became of the report: --delivered or --undelivered, one of them.")
	}
	dir, err := state.Dir()
	if err != nil {
		return usage("%v", err)
	}
	if os.Getenv(state.SessionEnv) != "" {
		// Run from a session's shell: only a verified main of this build
		// speaks for the person here. A shell outside every session is the
		// person's own.
		self, _, err := ownRun(dir)
		if errors.Is(err, errUpgraded) {
			return refuseUpgraded(dir, self)
		}
		if err != nil {
			return usage("only main or the person settles a stopped mailbox, and this session cannot be verified as main: %v. Ask main to run the line from its note.", err)
		}
		if self.Role != role.Main.ID {
			return usage("only main or the person settles a stopped mailbox; %s is a %s session. Ask main to run the line from its note.", self.Name, self.Role)
		}
	}
	model := settleModel{Session: name, Report: report, Delivered: delivered}
	wait, cancel := context.WithTimeout(context.Background(), readerLockWait)
	defer cancel()
	var settled error
	err = state.WithMailboxLock(wait, dir, name, func() error {
		model.Recorded, settled = inbox.Settle(wait, dir, name, report, delivered)
		return nil
	})
	if errors.Is(err, state.ErrMailboxBusy) {
		return failf("the mailbox of %s is busy right now; run rewake settle again in a moment", name)
	}
	if err != nil {
		return failf("could not lock the mailbox of %s: %v", name, err)
	}
	var refusal *inbox.SettleRefusal
	var stop *inbox.StoppedError
	switch {
	case errors.As(settled, &refusal):
		return usage("%s.", refusal.Reason)
	case errors.As(settled, &stop):
		model.Stopped = true
		for _, left := range stop.Reports {
			model.Remaining = append(model.Remaining, left.ID)
		}
	case settled != nil:
		return failf("the decision on %s is recorded, but the mailbox of %s could not be reconciled yet: %v; the next turn end or rewake settle looks again", report, name, settled)
	}
	return printValue(ctx, model, func() []string { return []string{settleLine(model)} })
}

func settleLine(model settleModel) string {
	word := "delivered"
	if !model.Delivered {
		word = "undelivered"
	}
	head := fmt.Sprintf("Rewake: %s settled %s", model.Report, word)
	if !model.Recorded {
		head = fmt.Sprintf("Rewake: %s was settled %s already", model.Report, word)
	}
	if model.Stopped {
		return fmt.Sprintf("%s; %s stays stopped on %s, settle each the same way.", head, model.Session, strings.Join(model.Remaining, ", "))
	}
	return fmt.Sprintf("%s; the mailbox of %s goes on.", head, model.Session)
}
