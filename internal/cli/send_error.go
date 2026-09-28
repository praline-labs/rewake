package cli

import "github.com/praline-labs/rewake/internal/inbox"

// errorKind is hook-only. With no public flag, a sender cannot impersonate a
// failed turn through send; error reports are routed by turn-ended.
var errorKind = messageKind{kind: inbox.Error, summary: "a failed turn, written only by the hook, owing no reply."}
