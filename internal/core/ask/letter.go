package ask

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
)

// maxShown bounds the call's input a letter shows: a file's whole content
// would bury the choices.
const maxShown = 2000

// ErrNoMain says the room has no main session to ask.
var ErrNoMain = errors.New("the room has no main session to answer the request")

// TellMain leaves main an ordinary notify from the worker naming the request,
// what it shows and how to answer it. No report is owed for it.
func TellMain(dir, worker, epoch, id string, offer Offer, deadline time.Time) error {
	return tell(dir, worker, epoch, Letter(worker, id, offer, deadline))
}

// TellMainDialog leaves main an ordinary notify that the worker waits at a
// dialog on its screen that no request carries, with what the harness said
// of it: main asks the person to answer or cancel it at the worker's pane.
func TellMainDialog(dir, worker, epoch, message string) error {
	return tell(dir, worker, epoch, fmt.Sprintf(
		"Rewake: %s waits at a dialog in its pane: %s\nrewake cannot answer this dialog for it: ask the person to answer or cancel it in %s's pane.",
		worker, message, worker))
}

// tell leaves the room's main a notify from the worker.
func tell(dir, worker, epoch, text string) error {
	sessions, err := registry.List(dir)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if role.Of(session.Role).ID != role.Main.ID || session.Name == worker {
			continue
		}
		return mail.Put(dir, mail.Message{
			ID: mail.NewID(), From: worker, FromEpoch: epoch, To: session.Name, ToEpoch: session.Epoch(),
			Kind: mail.Note, Text: text, CreatedAt: time.Now(),
		})
	}
	return ErrNoMain
}

// Letter is the text main reads: the request on its first line, then the
// call or the questions with their choices, the deadline and the command.
func Letter(worker, id string, offer Offer, deadline time.Time) string {
	var lines []string
	by := deadline.Local().Format("15:04:05")
	if offer.Kind == Permission {
		lines = append(lines,
			fmt.Sprintf("Rewake: %s asks permission for %s (request %s); answer by %s.", worker, offer.Tool, id, by),
			"Input: "+shown(offer.Input))
		if offer.ReadOnly {
			lines = append(lines, "Read-only: the tool says this call only reads; a read outside the project can still expose keys or logins.")
		}
		if offer.Cwd != "" {
			lines = append(lines, "In: "+offer.Cwd)
		}
		lines = append(lines,
			"Choices: once allows this call; session allows this exact call until the worker ends or its mod reloads; refuse [reason] refuses it, and the reason goes to its model.",
			fmt.Sprintf("Answer: rewake answer %s %s once", worker, id))
	} else {
		first := ""
		if len(offer.Questions) > 0 {
			first = ": " + offer.Questions[0].Text
		}
		lines = append(lines, fmt.Sprintf("Rewake: %s asks a question (request %s)%s Answer by %s.", worker, id, first, by))
		example := []string{"rewake answer", worker, id}
		for index, prompt := range offer.Questions {
			if len(offer.Questions) > 1 {
				lines = append(lines, fmt.Sprintf("Question %d: %s", index+1, prompt.Text))
			}
			for n, option := range prompt.Options {
				line := fmt.Sprintf("  %d %s", n+1, option.Label)
				if option.Description != "" {
					line += " — " + option.Description
				}
				lines = append(lines, line)
			}
			if prompt.Multi {
				lines = append(lines, "  several options may be chosen: name them as 1,3")
			}
			example = append(example, "1")
		}
		lines = append(lines,
			"Any other text is the answer typed under Other"+map[bool]string{true: "; quote each question's answer as one word.", false: "."}[len(offer.Questions) > 1],
			"Answer: "+strings.Join(example, " "))
	}
	return strings.Join(append(lines, "A person at the worker's screen may answer first; the first answer counts, and with none by the deadline the worker is refused."), "\n")
}

func shown(input string) string {
	runes := []rune(input)
	if len(runes) <= maxShown {
		return input
	}
	return string(runes[:maxShown]) + fmt.Sprintf(" … (%d characters more)", len(runes)-maxShown)
}
