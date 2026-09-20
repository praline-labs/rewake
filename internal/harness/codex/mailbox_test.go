package codex

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness/codex/gateway"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func decodedMailbox(t *testing.T, params map[string]json.RawMessage) gateway.MailboxNotice {
	t.Helper()
	if string(params["input"]) != "[]" {
		t.Fatalf("announcement became user input: %s", params["input"])
	}
	var output map[string]json.RawMessage
	if json.Unmarshal(params["toolOutput"], &output) != nil || len(output) != 2 || string(output["name"]) != `"rewake_mailbox_notice"` {
		t.Fatalf("invalid standalone output: %s", params["toolOutput"])
	}
	var text string
	var notice gateway.MailboxNotice
	if json.Unmarshal(output["output"], &text) != nil || json.Unmarshal([]byte(text), &notice) != nil || len(notice.Members) == 0 {
		t.Fatalf("invalid mailbox output: %s", params["toolOutput"])
	}
	return notice
}

func TestMailboxOnlyDescribesFixedReservedMembers(t *testing.T) {
	first := inbox.Message{ID: "first", From: "main", FromEpoch: "sender-epoch", To: "worker", ToEpoch: "receiver-epoch", Text: "first preview\nprivate task body"}
	second := first
	second.ID, second.Text = "second", "last preview\nother private body"
	batch := first
	batch.ID, batch.Batch = "group", []inbox.Message{first, second}
	notice := mailboxNotice(batch)
	want := []gateway.MailboxMember{{ID: first.ID, From: first.From, FromEpoch: first.FromEpoch, To: first.To, ToEpoch: first.ToEpoch}, {ID: second.ID, From: second.From, FromEpoch: second.FromEpoch, To: second.To, ToEpoch: second.ToEpoch}}
	if !reflect.DeepEqual(notice.Members, want) {
		t.Fatalf("members = %+v", notice.Members)
	}
	raw, err := json.Marshal(notice)
	if err != nil || strings.Contains(string(raw), "private") || !strings.Contains(notice.Notice, "2 new messages") {
		t.Fatalf("unexpected body or count: %s (%v)", raw, err)
	}
	single := mailboxNotice(first)
	if !reflect.DeepEqual(single.Members, want[:1]) {
		t.Fatalf("single identity lost: %+v", single)
	}
}
