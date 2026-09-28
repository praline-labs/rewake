package cli

import "github.com/praline-labs/rewake/internal/inbox"

// noteKind is a heads-up. Reading it asks for nothing back, so the session does
// not report its turn to the sender.
var noteKind = messageKind{
	kind:    inbox.Note,
	summary: "a heads-up.",
	flag:    Option{Flag: "--notify", Summary: "A heads-up that needs no answer; no report comes back."},
	wait:    defaultWait,
}
