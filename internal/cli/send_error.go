package cli

import mail "github.com/praline-labs/rewake/internal/inbox"

// errorKind is written only by the end of a turn. With no public flag, a
// sender cannot impersonate a failed turn through send.
var errorKind = messageKind{kind: mail.Error, summary: "a failed turn, written only by the end of a turn."}
