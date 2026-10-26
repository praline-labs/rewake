package cli

import "github.com/praline-labs/rewake/internal/core/mail"

// A stopped turn, an aborted one, asks peers to wait, not to send the task
// again.
var stoppedKind = messageKind{kind: mail.Stopped, summary: "a turn aborted at the keyboard or by a main's rewake interrupt; its text is the turn's partial answer or names the main that interrupted it, and may be empty. The task stays open and a later report may settle it: do not resend."}
