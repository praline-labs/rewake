package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

func TestPeekHasNoConsumptionOrDispatchAcknowledgement(t *testing.T) {
	dir, self, peer := stateCaller(t, "write")
	clock, err := inbox.OpenReadClock(context.Background(), dir, self.Name, self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	leaveUnread(t, dir, inbox.Message{From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Task, Text: "work"})
	for _, args := range [][]string{{"inbox", "--peek"}, {"inbox", "--json"}} {
		if Run(args, brokenWriter{}, brokenWriter{}) == ExitOK {
			t.Fatal("failed output reported success")
		}
	}
	for _, args := range [][]string{{"inbox", "--peek"}, {"inbox", "--peek", "--json"}} {
		if code, _, errOut := run(args...); code != ExitOK {
			t.Fatal(errOut)
		}
	}
	if clock.Snapshot().Through != 0 || len(inbox.Waiters(dir, self.Name, self.Epoch())) != 0 {
		t.Fatal("peek consumed task")
	}
	entries, err := os.ReadDir(state.InboxPath(dir, self.Name))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".overview-") {
			t.Fatal("peek wrote obsolete dispatch acknowledgement")
		}
	}
	if code, _, errOut := run("inbox"); code != ExitOK {
		t.Fatal(errOut)
	}
	if clock.Snapshot().Through != 1 || len(inbox.Waiters(dir, self.Name, self.Epoch())) != 1 {
		t.Fatal("actual read lost task boundary")
	}
}

func TestConcurrentPeeksAndFailedReadRecording(t *testing.T) {
	dir, self, peer := stateCaller(t, "write")
	leaveUnread(t, dir, inbox.Message{From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Task, Text: "work"})
	done := make(chan int, 2)
	for range 2 {
		go func() { code, _, _ := run("inbox", "--peek", "--json"); done <- code }()
	}
	for range 2 {
		if code := <-done; code != ExitOK {
			t.Fatal("concurrent peek failed")
		}
	}
	blocked := state.AwaitingPath(dir, self.Name)
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run("inbox"); code == ExitOK {
		t.Fatal("failed obligation recording reported success")
	}
	if unread, err := inbox.AvailableUnread(dir, self.Name, self.Epoch()); err != nil || len(unread) != 1 {
		t.Fatal("failed read lost unread task", err)
	}
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := run("inbox", "--peek"); code != ExitOK {
		t.Fatal("recovery peek failed")
	}
	if len(inbox.Waiters(dir, self.Name, self.Epoch())) != 0 {
		t.Fatal("peek created obligation")
	}
}

// The overview is one line of rewake's own and a row per message: how to read
// them is in the help, not repeated under every peek.
func TestPeekPrintsACountAndRowsOnly(t *testing.T) {
	dir, self, peer := stateCaller(t, "write")
	leaveUnread(t, dir, inbox.Message{From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Task, Text: "rerun the smoke"})
	code, out, errOut := run("inbox", "--peek")
	if code != ExitOK {
		t.Fatal(errOut)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 || lines[0] != "Rewake: 1 unread message:" || !strings.Contains(lines[1], " · "+peer.Name+" · task · ") || !strings.HasSuffix(lines[1], " · rerun the smoke") {
		t.Fatalf("overview = %q", out)
	}
}
