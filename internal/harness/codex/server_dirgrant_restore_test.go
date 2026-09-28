package codex

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/role"
)

// Two copies can name one directory for two grants: an earlier task's that
// has closed since, which main refuses, and a later one's still open, which
// it confirms. The confirmed grant holds the root whichever copy is read
// first: the refused one takes out only what no confirmed grant holds.
func TestARefusedHintKeepsARootAConfirmedOneHolds(t *testing.T) {
	for _, order := range [][]string{{"open", "closed"}, {"closed", "open"}} {
		t.Run(strings.Join(order, "-then-"), func(t *testing.T) {
			workspace, lib := t.TempDir(), t.TempDir()
			server, captured := gitDeliveryFixture(t, role.General,
				gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
			)
			dirGrantSession(t, server)
			server.epoch = runOf(t, os.Getpid())
			main := server.epoch
			previous := resumedMain(t, server, map[string]string{"open": lib, "closed": lib}, "closed")
			waiting(t, server, "open", true)
			var copied []grant.Entry
			for _, id := range order {
				copied = append(copied, grant.Entry{Path: lib, Message: id, Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main})
			}
			if err := grant.Save(server.mailbox, server.name, previous, copied); err != nil {
				t.Fatal(err)
			}

			roots, result := deliverResumed(t, server, captured, inbox.Message{ID: "n1", Kind: inbox.Note})
			if result.State != inbox.Delivered || !slices.Equal(roots, []string{workspace, lib}) {
				t.Fatalf("roots=%q result=%+v", roots, result)
			}
			if !strings.Contains(result.Detail, "restored after the resume, confirmed again by lead: "+lib) || strings.Contains(result.Detail, "taken back") {
				t.Fatalf("the detail: %s", result.Detail)
			}
			entries := journal(server)
			if got := outcomes(entries); !slices.Equal(got, []string{lib + "=granted"}) || entries[0].Message != "open" {
				t.Fatalf("journal = %+v", entries)
			}
		})
	}
}

// Each main may take most of its exchange to answer, longer together than a
// reservation lives. They are asked before the notice holds one, and all at
// once: the notice waits meanwhile, and then goes with every grant restored
// rather than failing for good.
func TestSlowConfirmationsDoNotFailTheNotice(t *testing.T) {
	workspace, lib, doc := t.TempDir(), t.TempDir(), t.TempDir()
	server, captured := gitDeliveryFixture(t, role.General,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
	)
	dirGrantSession(t, server)
	server.epoch = runOf(t, os.Getpid())
	main := server.epoch
	previous := slowResumedMain(t, server, map[string]string{"m1": lib, "m2": doc}, 1600*time.Millisecond)
	waiting(t, server, "m1", true)
	waiting(t, server, "m2", true)
	copied := []grant.Entry{
		{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main},
		{Path: doc, Message: "m2", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main},
	}
	if err := grant.Save(server.mailbox, server.name, previous, copied); err != nil {
		t.Fatal(err)
	}

	roots, result := deliverResumed(t, server, captured, inbox.Message{ID: "n1", Kind: inbox.Note})
	if result.State != inbox.Delivered || !slices.Equal(roots, []string{workspace, lib, doc}) && !slices.Equal(roots, []string{workspace, doc, lib}) {
		t.Fatalf("roots=%q result=%+v", roots, result)
	}
	if got := len(journal(server)); got != 2 {
		t.Fatalf("journal = %+v", journal(server))
	}
}

// A reservation that lapsed before its notice went out sent nothing: the
// notice stays pending, to go again, rather than failing for good.
func TestANoticeNotSentStaysPending(t *testing.T) {
	server, captured := gitDeliveryFixture(t, role.General)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	message := inbox.Message{ID: "n1", Kind: inbox.Note, Text: "later"}
	reserved, err := server.Reserve(ctx, message)
	if err != nil {
		t.Fatal(err)
	}
	defer reserved.Close()
	result := reserved.(inbox.CheckedAnnouncer).DeliverChecked(ctx, message, func() bool {
		<-ctx.Done()
		return true
	})
	if result.State != inbox.Pending {
		t.Fatalf("result = %+v", result)
	}
	select {
	case params := <-captured:
		t.Fatalf("a notice went out: %v", params)
	default:
	}
}
