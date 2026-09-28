package cli

import "github.com/praline-labs/rewake/internal/inbox"

// interimKind is written only by the end of a turn marked with rewake pending.
// With no public flag, a sender cannot fake one through send.
var interimKind = messageKind{kind: inbox.Interim, summary: "an interim turn end: the work is still going, and its report follows; owes no reply, and a --question keeps waiting."}
