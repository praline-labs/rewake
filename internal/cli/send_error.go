package cli

import "github.com/praline-labs/rewake/internal/core/mail"

// errorKind is written only by the end of a turn. With no public flag, a
// sender cannot impersonate a failed turn through send.
var errorKind = messageKind{kind: mail.Error, summary: "a failed turn, written only by the end of a turn; its text says why, or that the turn gave no answer and no reason. An error that answers a read task or question settles it and no later report comes: check rewake inbox --awaited and the worker's saved work, and send a new task if the work is to go on."}
