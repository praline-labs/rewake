package codex

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/role"
)

// A grant restored at one notice holds its root at the next ones too. Two
// copies name one directory: the grant of a main that confirms it, and one
// from a main that does not answer yet. The first notice restores the one
// and leaves the other for later; when that main has ended and the next
// notice finds its hint refused, the directory stays, since a grant journaled
// in this conversation holds it.
func TestAHintRefusedLaterKeepsARootRestoredEarlier(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	server, captured := gitDeliveryFixture(t, role.General,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, lib}, "idle")},
	)
	dirGrantSession(t, server)
	server.epoch = runOf(t, os.Getpid())
	main := server.epoch
	previous := resumedMain(t, server, map[string]string{"open": lib})
	waiting(t, server, "open", true)
	quiet := exec.Command("sleep", "60")
	if err := quiet.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = quiet.Process.Kill(); _ = quiet.Wait() })
	copied := []grant.Entry{
		{Path: lib, Message: "open", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main},
		{Path: lib, Message: "quiet", Outcome: grant.Granted, Thread: fixtureRoot, From: "other", FromEpoch: runOf(t, quiet.Process.Pid)},
	}
	if err := grant.Save(server.mailbox, server.name, previous, copied); err != nil {
		t.Fatal(err)
	}

	roots, result := deliverResumed(t, server, captured, inbox.Message{ID: "n1", Kind: inbox.Note})
	if result.State != inbox.Delivered || !slices.Equal(roots, []string{workspace, lib}) {
		t.Fatalf("first notice: roots=%q result=%+v", roots, result)
	}

	_ = quiet.Process.Kill()
	_ = quiet.Wait()
	roots, result = deliverResumed(t, server, captured, inbox.Message{ID: "n2", Kind: inbox.Note})
	if result.State != inbox.Delivered || roots != nil || strings.Contains(result.Detail, "taken back") {
		t.Fatalf("second notice: roots=%q result=%+v", roots, result)
	}
	if got := outcomes(journal(server)); !slices.Equal(got, []string{lib + "=granted"}) {
		t.Fatalf("journal = %v", got)
	}
}

// Before a resume has finished no conversation is selected, and Reserve waits
// for one. The answers a notice needs are those of the conversation it then
// reserves: the notice waits for them, rather than going out without the
// grants the conversation had.
func TestTheConversationReservedIsTheOneConfirmed(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	server, captured, start := gitDeliveryFixtureUnstarted(t, role.General, "idle", false, nil, nil,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace}, "idle")},
	)
	dirGrantSession(t, server)
	server.epoch = runOf(t, os.Getpid())
	main := server.epoch
	previous := resumedMain(t, server, map[string]string{"m1": lib})
	waiting(t, server, "m1", true)
	copied := []grant.Entry{{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main}}
	if err := grant.Save(server.mailbox, server.name, previous, copied); err != nil {
		t.Fatal(err)
	}

	first := make(chan inbox.Result, 1)
	go func() {
		first <- server.Deliver(context.Background(), inbox.Message{ID: "n1", Kind: inbox.Note, Text: "later"})
	}()
	time.Sleep(200 * time.Millisecond)
	start()
	if result := <-first; result.State != inbox.Pending || !strings.Contains(result.Detail, resumeWait) {
		t.Fatalf("the first notice went without waiting: %+v", result)
	}
	select {
	case params := <-captured:
		t.Fatalf("a notice went out: %v", params)
	default:
	}
	roots, result := deliverResumed(t, server, captured, inbox.Message{ID: "n1", Kind: inbox.Note})
	if result.State != inbox.Delivered || !slices.Equal(roots, []string{workspace, lib}) {
		t.Fatalf("roots=%q result=%+v", roots, result)
	}
}

// A reservation that lapsed before its notice was prepared made nothing
// readable and sent nothing: preparing it, through the inbox or in the
// delivery itself, leaves the notice pending rather than failing it.
func TestAReservationLapsedBeforePreparingKeepsTheNoticePending(t *testing.T) {
	server, captured := gitDeliveryFixture(t, role.General)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	message := inbox.Message{ID: "n1", Kind: inbox.Note, Text: "later"}
	reserved, err := server.Reserve(ctx, message)
	if err != nil {
		t.Fatal(err)
	}
	defer reserved.Close()
	<-ctx.Done()
	time.Sleep(50 * time.Millisecond)

	called := false
	err = reserved.Prepare(func(string) error { called = true; return nil })
	if called || !errors.Is(err, inbox.ErrNotYet) || errors.Is(err, inbox.ErrThreadUnavailable) {
		t.Fatalf("preparing through the inbox: called=%v err=%v", called, err)
	}
	result := reserved.(inbox.CheckedAnnouncer).DeliverChecked(ctx, message, func() bool { return true })
	if result.State != inbox.Pending {
		t.Fatalf("result = %+v", result)
	}
	select {
	case params := <-captured:
		t.Fatalf("a notice went out: %v", params)
	default:
	}
}
