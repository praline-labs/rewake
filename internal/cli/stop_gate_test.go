package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A journal that cannot be read stops the mailbox for every call that would
// change it, not only for the turn end that found it (8-stop): a pending mark
// or a read taken meanwhile would change what the stopped end decides.
// Removing the journal is no evidence of what it would have done: the calls
// stay refused, naming the removed path. The same journal with valid bytes,
// closed to reading and opened again, is, and the calls go on.
func TestAnUnreadableJournalStopsEveryCall(t *testing.T) {
	for _, bytes := range []string{"invalid, removed", "valid, closed and reopened"} {
		t.Run(bytes, func(t *testing.T) {
			dir, self, web := peerSession(t)
			readKind(t, dir, web, mail.Task)
			unread := rawUnread(t, dir, "api", map[string]any{"from": "web", "fromEpoch": web.Epoch(), "toEpoch": self.Epoch(), "kind": string(mail.Task), "text": "arrives while stopped"})
			if err := os.MkdirAll(mail.JournalPath(dir, "api"), 0o700); err != nil {
				t.Fatal(err)
			}
			broken := filepath.Join(mail.JournalPath(dir, "api"), "broken")
			valid := bytes != "invalid, removed"
			if valid {
				if err := mail.WriteJournal(dir, "api", "broken", mail.TurnJournal{Epoch: self.Epoch(), Op: "end"}); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(broken, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(broken, 0o600) })
			} else if err := os.WriteFile(broken, []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
			if completeTurn(dir, self, mail.TurnEnd{Text: "answer", Ended: 10}, "") == nil {
				t.Fatal("the turn end went on past an unreadable journal")
			}
			turnStarted(t, dir, self, markAt-1)
			for _, words := range [][]string{{"pending", "waiting"}, {"inbox"}} {
				if code, out, errOut := run(words...); code != ExitFailed {
					t.Fatalf("%s on a stopped mailbox: %d %s %s", words[0], code, out, errOut)
				}
			}
			if owes(t, dir, self, unread) {
				t.Fatal("a stopped mailbox was read")
			}
			if valid {
				if err := os.Chmod(broken, 0o600); err != nil {
					t.Fatal(err)
				}
				if code, out, errOut := run("pending", "waiting on"); code != ExitOK {
					t.Fatalf("pending once the journal reads: %d %s %s", code, out, errOut)
				}
				return
			}
			if err := os.Remove(broken); err != nil {
				t.Fatal(err)
			}
			if code, out, errOut := run("pending", "waiting on"); code != ExitFailed || !strings.Contains(errOut, broken+" was removed while stopped") {
				t.Fatalf("pending once the journal is removed: %d %s %s", code, out, errOut)
			}
		})
	}
}

// The evidence an end's effects decide by stops every later call while it
// is unknown, not only the end that met it: a kept answer that does not read
// leaves pending, a read and its parts refused.
func TestUnknownEvidenceStopsTheCallsAfterTheEnd(t *testing.T) {
	dir, self, web := peerSession(t)
	readKind(t, dir, web, mail.Task)
	version := "v1"
	report := mail.Message{ID: mail.NewID(), From: "api", FromEpoch: self.Epoch(), To: "web", ToEpoch: web.Epoch(), Kind: mail.Finished, Text: "answer"}
	kept := filepath.Join(state.InboxPath(dir, "api"), "pending", "kept.json")
	if err := os.MkdirAll(filepath.Dir(kept), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := mail.WriteJournal(dir, "api", "end", mail.TurnJournal{Epoch: self.Epoch(), Op: "end", Ended: 100, Reports: []mail.Message{report}, Kept: &version}); err != nil {
		t.Fatal(err)
	}
	if completeTurn(dir, self, mail.TurnEnd{Text: "another answer", Ended: 200}, "") == nil {
		t.Fatal("the end went on past an unknown kept answer")
	}
	turnStarted(t, dir, self, markAt-1)
	for _, words := range [][]string{{"pending", "waiting"}, {"inbox"}} {
		if code, _, errOut := run(words...); code != ExitFailed || !strings.Contains(errOut, "kept.json") {
			t.Fatalf("%s past an unknown kept answer: %d %s", words[0], code, errOut)
		}
	}
}
