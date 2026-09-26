package codex

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

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

// A recall's member entry names the message not to act on: the recall's own
// id names only the note, which tells an agent reading the members nothing.
func TestAMailboxRecallMemberNamesTheWithdrawnMessage(t *testing.T) {
	task := inbox.Message{ID: "task", From: "main", FromEpoch: "sender-epoch", To: "worker", ToEpoch: "receiver-epoch", Text: "neighbor"}
	recall := task
	recall.ID, recall.Text, recall.Recall = "recall", "Do not act on task withdrawn from main (12:00:00): withdrawn unread.", &inbox.RecallNotice{ID: "withdrawn", Kind: inbox.Task}
	batch := task
	batch.ID, batch.Batch = "group", []inbox.Message{task, recall}
	notice := mailboxNotice(batch)
	if len(notice.Members) != 2 || notice.Members[0].Recalls != "" || notice.Members[1].Recalls != "withdrawn" {
		t.Fatalf("members = %+v", notice.Members)
	}
	if !strings.Contains(notice.Notice, "\n  ↳ Do not act on task withdrawn") {
		t.Fatalf("notice = %q", notice.Notice)
	}
	if single := mailboxNotice(recall); len(single.Members) != 1 || single.Members[0].Recalls != "withdrawn" {
		t.Fatalf("single = %+v", single.Members)
	}
}

// A replacement member names the message it replaces, which the notice shows
// only by its short form; its neighbors carry no such link.
func TestAMailboxReplacementMemberNamesTheReplacedMessage(t *testing.T) {
	older := inbox.Message{ID: "task", From: "main", FromEpoch: "sender-epoch", To: "worker", ToEpoch: "receiver-epoch", Text: "neighbor", CreatedAt: time.Unix(1, 0)}
	replacement := older
	replacement.ID, replacement.Text, replacement.Replaces = "replacement", "use the replica", "100-5ca1ab1e0001"
	newer := older
	newer.ID, newer.Text, newer.CreatedAt = "newer", "build finished", time.Unix(2, 0)
	batch := older
	batch.ID, batch.Batch = "group", []inbox.Message{older, replacement, newer}
	notice := mailboxNotice(batch)
	if len(notice.Members) != 3 || notice.Members[0].Replaces != "" || notice.Members[1].Replaces != "100-5ca1ab1e0001" || notice.Members[2].Replaces != "" {
		t.Fatalf("members = %+v", notice.Members)
	}
	if !strings.Contains(notice.Notice, "\n  ↳ main task: Replaces 5ca1ab1e0001 (withdrawn): use the replica\n  ↳ main task: build finished") {
		t.Fatalf("notice = %q", notice.Notice)
	}
}
