package workflow

// What the fixture accepts as a delivery, and the payload it accepts it into.
//
// Split from codexshim_turn_test.go when that file passed the project's
// 400-line limit. The two subjects the file named are now one each: the fork
// between starting and steering a turn, with the lifecycle of the turn itself,
// stayed there; what a turn/start must look like before any of that begins is
// here.
//
// Nothing about a delivery is decided anywhere else. A fixture that accepted
// any turn at all would let a scenario pass while the notice was mangled on
// the way, so what arrives is checked: the operation's name, the message
// identities, the epochs.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// mailboxNotice mirrors the payload the wrapper sends. It is spelled out here
// rather than imported: test/workflow is outside internal, and a fixture that
// shared the product's struct would agree with it by construction.
type mailboxNotice struct {
	Notice  string `json:"notice"`
	Members []struct {
		ID        string `json:"id"`
		From      string `json:"from"`
		FromEpoch string `json:"fromEpoch"`
		To        string `json:"to"`
		ToEpoch   string `json:"toEpoch"`
	} `json:"members"`
}

// checkedDelivery answers what a delivery must look like, and nothing else: no
// state is changed and no work begins. Everything a scenario relies on when it
// says "a turn was accepted" is decided here.
func (s *shimSession) checkedDelivery(params json.RawMessage) (mailboxNotice, error) {
	var asked struct {
		ThreadID   string `json:"threadId"`
		MessageID  string `json:"clientUserMessageId"`
		ToolOutput struct {
			Name   string `json:"name"`
			Output string `json:"output"`
		} `json:"toolOutput"`
	}
	var notice mailboxNotice
	// The description decides first, and the struct is filled from a request
	// it has already accepted. Reading into the struct first and checking
	// afterwards is what let the two views of one request disagree: a repeated
	// key kept the earlier object's fields in the struct and the later one's
	// in the map, and a required field went missing between them.
	output, _ := outputOf(params)
	if wrong := unservedDelivery(params, output); wrong != "" {
		return notice, errors.New(wrong)
	}
	if json.Unmarshal(params, &asked) != nil {
		return notice, errors.New("unreadable turn/start params")
	}
	if asked.ThreadID != s.thread {
		return notice, fmt.Errorf("turn for a conversation this server does not have: %q", asked.ThreadID)
	}
	// Presence and kind of the input list are the description's business; what
	// is left here is emptiness, which no shape can state. A delivery is the
	// case where the list is there and empty, because the notice is the whole
	// message and any text would mean something else was sent.
	input, _ := rawField(params, "input")
	var carried []json.RawMessage
	if json.Unmarshal(input, &carried) != nil || len(carried) != 0 {
		return notice, errors.New("a mailbox delivery carries no input")
	}
	if asked.ToolOutput.Name != mailboxToolName {
		return notice, fmt.Errorf("turn output is from %q, not the mailbox", asked.ToolOutput.Name)
	}
	if asked.MessageID == "" {
		return notice, errors.New("a delivery names the message it carries")
	}
	if json.Unmarshal([]byte(asked.ToolOutput.Output), &notice) != nil {
		return notice, errors.New("unreadable mailbox notice")
	}
	if len(notice.Members) == 0 {
		return notice, errors.New("a mailbox notice with no members")
	}
	for _, member := range notice.Members {
		if member.ID == "" || member.From == "" || member.To == "" {
			return notice, errors.New("a notice member without identities")
		}
		// Both epochs, not just the recipient's. The sender's epoch is what
		// decides which run of the sender a report goes back to, so a delivery
		// that lost it would be a report addressed to nobody.
		if member.FromEpoch == "" || member.ToEpoch == "" {
			return notice, errors.New("a notice member without epochs")
		}
		// The real server has no idea who the mail is for, so this is not the
		// server's check — but the wrapper runs this session and tells it its
		// own name and epoch, so a delivery addressed elsewhere is free to
		// catch here, and mail landing in the wrong session is the failure
		// this scenario exists to rule out.
		if member.To != os.Getenv(sessionNameEnv) || member.ToEpoch != os.Getenv(sessionEpochEnv) {
			return notice, fmt.Errorf("a delivery addressed to %s/%s reached %s/%s",
				member.To, member.ToEpoch, os.Getenv(sessionNameEnv), os.Getenv(sessionEpochEnv))
		}
	}
	return notice, nil
}

// outputOf reads the notice out of a request, without deciding anything about
// it. It answers "" for a request it cannot read that far into — a missing
// field, a field of the wrong kind, params that are not an object — and every
// one of those is refused by the description, which runs on the same request.
// Nothing here treats "could not read it" as "there was nothing to read".
func outputOf(params json.RawMessage) (string, bool) {
	output, ok := rawField(params, "toolOutput")
	if !ok || jsonKind(output) != "object" {
		return "", false
	}
	raw, ok := rawField(output, "output")
	if !ok || jsonKind(raw) != "string" {
		return "", false
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return "", false
	}
	// The second answer is not decoration: a notice that is present and empty
	// and one that could not be read are different requests, and the caller
	// refuses both — but for different reasons, and a reason is what a person
	// debugging a refusal has to go on.
	return text, true
}
