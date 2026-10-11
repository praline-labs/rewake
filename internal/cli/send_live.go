package cli

import (
	"github.com/praline-labs/rewake/internal/inbox"
)

// What a send does by the recipient's live set, the capabilities its run has
// proven as its session record carries them
// (docs/v2/design-api.md#two-levels-offered-and-live): never what its harness
// offers, which says only what a run of it could prove.

// unwoken is the detail of a letter written to a run without a live Wake.
const unwoken = "accepted; no notice can reach this session, which reads its mail when it runs rewake inbox"

// reportUnwoken answers a letter written to a run whose Wake is not live:
// accepted, exit 3, without waiting for a delivery no notice can make. The
// letter stays in the mailbox, read when the session runs rewake inbox, and
// noticed if a Wake becomes live before then.
func reportUnwoken(ctx *Context, after sent, message inbox.Message) error {
	model := sendModel{
		ID: message.ID, To: after.target.Name, From: message.From,
		GrantGit: message.GrantGit, GrantDirs: message.GrantDirs, AlreadyWritable: after.writable,
		Addenda: after.addenda, Replaces: message.Replaces, Named: after.named,
		State: string(inbox.Pending), Detail: unwoken,
	}
	if ctx.op != nil {
		model.Receipt = ctx.op.record.Token
	}
	return printDelivery(ctx, after.target, model)
}
