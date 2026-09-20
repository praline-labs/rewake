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
}
