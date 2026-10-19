package cli

import (
	"errors"

	"github.com/praline-labs/rewake/internal/core/receipt"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// retryCommand is retry, which finishes or shows again what a call with a
// receipt began (docs/archive/mail-bridge.md#receipts-retries-and-deadlines). A call
// that stopped short, or whose answer was lost, leaves its receipt in the answer
// it did give or in the refusal of the call that repeated it; this is the one
// way back to that operation that cannot start a second one.
func retryCommand() *Command {
	return &Command{
		Name:           "retry",
		Args:           "<receipt>",
		MaxPositionals: 1,
		Summary:        "Finish, or show again, what an earlier call with this receipt began: a heads-up or a pending mark.",
		Options:        []Option{jsonOption},
		Examples:       []string{"rewake retry 0123456789abcdef01234567", "rewake retry 0123456789abcdef01234567 --json"},
		Notes: []string{
			"A receipt belongs to the run that printed it and is kept for a day after its operation finished.",
			"--json prints the answer's model, whatever form the first call printed; without it the answer comes in the form of the first call.",
			"A finished heads-up or mark prints its answer again and does nothing more; an unfinished one takes its remaining steps, to the same letter id and the same recipient run as the first attempt.",
		},
		Access:  changeForm,
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
	original, err := ctx.table.parse(record.Words)
	if err != nil || original.Call.Command == nil {
		return failf("the receipt %s holds words this rewake cannot run again", token)
	}
	handler := original.Call.Command.Handler
	switch original.Call.Command.Name {
	case "send", "pending":
	default:
		return failf("the receipt %s is of %s, which has nothing to finish", token, original.Call.Command.Name)
	}
	// The retry's own --json asks for the model of the answer, which the
	// record keeps; without it the operation answers as it first did.
	ctx.JSON = ctx.JSON || original.Call.Switch("json")
	ctx.journaling = true
	return runOperation(ctx, original.Call, handler, site, token, false)
}
