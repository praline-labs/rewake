package cli

import (
	"errors"
	"fmt"

	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// retryCommand is retry, which finishes or shows again what a call with a
// receipt began (docs/mail-bridge.md#receipts-retries-and-deadlines). A tool
// call that timed out, or whose answer the harness lost, leaves its receipt in
// the answer it did give or in the refusal of the call that repeated it; this
// is the one way back to that operation that cannot start a second one.
func retryCommand() *Command {
	return &Command{
		Name:           "retry",
		Args:           "<receipt>",
		MaxPositionals: 1,
		Summary:        "Finish, or show again, what an earlier call with this receipt began: a heads-up, a pending mark, a read or a long output.",
		Options:        []Option{jsonOption},
		Examples:       []string{"rewake retry 0123456789abcdef01234567", "rewake retry 0123456789abcdef01234567 --json"},
		Notes: []string{
			"A receipt belongs to the run that printed it and is kept for a day after its operation finished.",
			"A finished heads-up or mark prints its answer again and does nothing more; an unfinished one takes its remaining steps, to the same letter id and the same recipient run as the first attempt.",
			"A read shows again from its first part, the letters as the read froze them; they become read as its parts reach you. Letters that came since: rewake inbox --peek.",
			"Through the rewake tool it finishes only what the tool's own words may do; a receipt of a shell call, such as a heads-up whose text came from stdin, is finished in the shell.",
		},
		Handler: handleRetry,
	}
}

func handleRetry(ctx *Context, call Call) error {
	token := ""
	if len(call.Positionals) == 1 {
		token = call.Positionals[0]
	}
	if !receipt.ValidToken(token) {
		return &UsageError{Command: call.Command, Message: "retry needs the receipt an earlier call printed: 24 hexadecimal characters."}
	}
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	self, epoch, err := ownRun(dir)
	if errors.Is(err, errUpgraded) {
		return refuseUpgraded(dir, self)
	}
	if err != nil {
		return failf("%v; a receipt belongs to the run that printed it", err)
	}
	site := readSite{dir: dir, self: self, epoch: epoch}
	record, err := receipt.Load(dir, self.Name, epoch, token)
	if errors.Is(err, receipt.ErrUnknown) {
		return failf("no operation of this run answers to the receipt %s: it belongs to the run that printed it and is kept for a day", token)
	}
	if err != nil {
		return failf("could not read the receipt %s: %v", token, err)
	}
	// A retry through the tool is held to the tool's surface like any other
	// call: the words a shell call journaled — a text from stdin, say — are
	// the shell's to finish.
	if ctx.scope != nil {
		if _, err := ToolWords(record.Words); err != nil {
			return failf("the receipt %s holds words the rewake tool does not run (%v); finish it in the shell: rewake retry %s", token, err, token)
		}
	}
	switch {
	case record.Output == nil && len(record.Words) > 0 && record.Words[0] == "inbox":
		if record.Read != nil {
			if ctx.JSON && !record.Read.JSON {
				return &UsageError{Command: call.Command, Message: "this read was printed as text and shows again so; drop --json."}
			}
			ctx.JSON = record.Read.JSON
		}
		return emitRead(ctx, site, token, 0, 0)
	case record.Output != nil:
		if ctx.JSON && !record.Output.JSON {
			return &UsageError{Command: call.Command, Message: "this output was printed as text and shows again so; drop --json."}
		}
		_, err := fmt.Fprint(ctx.Stdout, renderOutputPart(record, 0, len(record.Output.Parts)))
		return err
	}
	original, err := parse(record.Words)
	if err != nil || original.Call.Command == nil {
		return failf("the receipt %s holds words this rewake cannot run again", token)
	}
	handler := original.Call.Command.Handler
	switch original.Call.Command.Name {
	case "send", "pending":
	default:
		return failf("the receipt %s is of %s, which has nothing to finish", token, original.Call.Command.Name)
	}
	// The operation answers as it was asked to: its first answer, replayed,
	// is in that form already.
	ctx.JSON = original.Call.Switch("json")
	ctx.journaling = true
	return runOperation(ctx, original.Call, handler, site, token)
}
