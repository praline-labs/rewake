package cli

import "github.com/iiiokojiadbi/rewake/internal/inbox"

// noteKind is a heads-up. Reading it asks for nothing back, so the session does
// not report its turn to the sender.
var noteKind = messageKind{
	kind: inbox.Note,
	flag: Option{Flag: "--notify", Summary: "A heads-up that needs no answer; the session will not report back."},
	wait: defaultWait,
}
