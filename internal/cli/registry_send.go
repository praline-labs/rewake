package cli

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/host/delivery"
)

// sendCommand is send's entry in the command table.
func sendCommand(catalog adapter.Catalog) *Command {
	return &Command{
		Name:           "send",
		Args:           "<name> <text>",
		MaxPositionals: 2,
		Summary:        "Give a session a task, a question or a heads-up. Use - as the text to read it from stdin.",
		Options: append(kindOptions(),
			Option{Flag: "--wait", Value: "<seconds>", Summary: "How long to wait for the delivery, default 5, or for a question's answer, default 600, of which the delivery takes 5 at most."},
			Option{Flag: "--to", Value: "<id>", Summary: "Add to an unreported task or question this run sent: a task of its own, announced at once, shown with it by rewake inbox --owed and settled by the same report. Takes a unique prefix of the id; an id rewake edit replaced adds to the replacement."},
			jsonOption,
		),
		Examples: sendExamples(catalog),
		Next:     []string{"rewake inbox"},
		Notes: []string{
			"A task is the default: the session works and ends its turn, and its final message comes back to you as a \"Rewake: <session> finished\" line.",
			"A question blocks until that final message and prints it; run a long one in the background.",
			fmt.Sprintf("A notify waits up to %s to share one notice with other mail, and send waits with it; a task or a question is announced at once.", delivery.Coalescing.Cap),
			"A verified main sees the answering session's state line above a question's answer; rewake list --help says what it holds.",
			"Quote the text as one argument: loose words are refused, not joined.",
			"The output ends with the message's id, which withdraw, edit and --to take whole or as a unique prefix of it or of its part after the dash.",
		},
		Access:  changeForm,
		Handler: handleSend,
	}
}
