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
			Option{Flag: "--brief", Value: "<id>", Summary: "Main only, with a task or a --question: give this brief of rewake brief list with it. The recipient's run may then read, tick and comment it, and reads its text with the letter."},
			Option{Flag: "--review", Summary: "With --brief: give the brief to review — the recipient's run reads and comments it but never ticks. Giving it again without --review makes it the run's work, and the other way round."},
			Option{Flag: "--commit", Value: "<rev>", Summary: "With --brief --review: give the reviewer a detached checkout of this commit, branch or tag of the repository main runs in, under the worktree root; the letter names its path. It is removed when the brief closes or the reviewer's run ends, unless it holds changes."},
			jsonOption,
		),
		Examples: sendExamples(catalog),
		Next:     []string{"rewake inbox"},
		Notes: []string{
			"A task is the default: when a session sends it to a reporting worker, the worker's final message comes back as a \"Rewake: <session> finished\" line when it ends its turn. Main owes no successful report, and a plain shell has no mailbox for a reply: use --notify when no answer is required.",
			"A question blocks until that final message and prints it; with no answer within --wait it exits 3, and the answer comes later as its report. Run a long one in the background.",
			fmt.Sprintf("A notify waits up to %s to share one notice with other mail, and send waits with it; a task or a question is announced at once.", delivery.Coalescing.Cap),
			"A verified main sees the answering session's state line above a question's answer; rewake list --help says what it holds.",
			"Quote the text as one argument: loose words are refused, not joined.",
			"The output ends with the message's id, which withdraw, edit and --to take whole or as a unique prefix of it or of its part after the dash.",
		},
		Access:  changeForm,
		Handler: handleSend,
	}
}
