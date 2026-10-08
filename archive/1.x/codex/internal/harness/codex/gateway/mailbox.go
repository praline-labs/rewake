package gateway

// MailboxTool names the real wrapper operation, not a native tool invocation.
const MailboxTool = "rewake_mailbox_notice"

// MailboxNotice describes reserved mail. It contains no task bodies and makes no
// claim that the recipient has consumed them; only an inbox read does that.
type MailboxNotice struct {
	Notice  string          `json:"notice"`
	Members []MailboxMember `json:"members"`
}

// MailboxMember keeps sender and recipient epochs with each durable message ID.
type MailboxMember struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	FromEpoch string `json:"fromEpoch"`
	To        string `json:"to"`
	ToEpoch   string `json:"toEpoch"`
	// Recalls is the id of the message a recall tells the recipient not to
	// act on: the recall's own id names only the note.
	Recalls string `json:"recalls,omitempty"`
	// Replaces is the id of the message a replacement sent by rewake edit
	// took the place of; the notice names it only by its short form.
	Replaces string `json:"replaces,omitempty"`
}
