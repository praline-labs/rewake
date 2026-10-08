package cli

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/sessionstate"
	"github.com/praline-labs/rewake/internal/state"
)

// The shell observation (docs/mail-bridge-channel.md#the-shell-observation):
// every mail command against every channel record, with and without a
// recipient's mailbox the command cannot write, judged by the rule: a mail
// operation that wrote is the shell working, one that failed reaching the
// state is the shell failing with its class, anything else no evidence; and
// none of it while the tool works.

func TestEveryShellCommandLeavesItsEvidence(t *testing.T) {
	now := channel.Stamp{Boot: boottime.Now(), Wall: time.Now()}
	records := map[string]*channel.Record{
		"no record": nil,
		"working":   {Tool: channel.ToolWorking},
		"failing":   {Tool: channel.ToolFailing, Class: channel.ClassServerGone, Interval: now},
		"no tool":   {Tool: channel.ToolNone, Reason: "no tool offered", Interval: now},
	}
	commands := []struct {
		name string
		argv []string
		// writes says the command, when it succeeds, writes the state;
		// toWeb says it writes web's mailbox, which a fault makes read-only.
		writes, toWeb bool
	}{
		{"send", []string{"send", "web", "hello"}, true, true},
		{"inbox", []string{"inbox"}, true, false},
		{"inbox --peek", []string{"inbox", "--peek"}, false, false},
		{"list", []string{"list"}, false, false},
		{"send to nobody", []string{"send", "nobody", "hello"}, false, false},
	}
	cases := 0
	for recordName, record := range records {
		for _, command := range commands {
			for _, fault := range []bool{false, true} {
				t.Run(recordName+"/"+command.name+map[bool]string{true: "/read-only"}[fault], func(t *testing.T) {
					dir := liveSession(t, "api")
					otherRun(t, dir, "web")
					epoch := epochOf(t, dir, "api")
					t.Setenv(state.SessionEnv, "api")
					t.Setenv(state.EpochEnv, epoch)
					leaveUnread(t, dir, inbox.Message{From: "web", To: "api", ToEpoch: epoch, Text: "a letter"})
					if err := sessionstate.Save(dir, "api", epoch, sessionstate.Snapshot{Channel: record, PublishedBoot: boottime.Now()}); err != nil {
						t.Fatal(err)
					}
					if fault {
						web := state.InboxPath(dir, "web")
						if err := state.EnsureSubdir(web); err != nil {
							t.Fatal(err)
						}
						if err := os.Chmod(web, 0o500); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = os.Chmod(web, 0o700) })
					}
					code, _, errOut := run(command.argv...)
					notes := receipt.TakeShell(dir, "api", epoch)

					var want *receipt.ShellNote
					switch {
					case record != nil && record.Working():
					case fault && command.toWeb:
						want = &receipt.ShellNote{Class: receipt.ShellNotAllowed}
					case command.writes:
						want = &receipt.ShellNote{OK: true}
					}
					switch {
					case want == nil && len(notes) != 0:
						t.Fatalf("exit %d: an observation %+v where none is evidence (%s)", code, notes, errOut)
					case want != nil && (len(notes) != 1 || notes[0].OK != want.OK || notes[0].Class != want.Class):
						t.Fatalf("exit %d: observations %+v, want %+v (%s)", code, notes, *want, errOut)
					}
				})
				cases++
			}
		}
	}
	t.Logf("%d shell calls", cases)
}

// A command that wrote and then waited reports its write's time: an interval
// that opened during the wait is not confirmed by it.
func TestTheShellEvidenceKeepsTheTimeOfItsWrite(t *testing.T) {
	dir := liveSession(t, "api")
	epoch := epochOf(t, dir, "api")
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(state.EpochEnv, epoch)
	state.ResetReach()
	t.Cleanup(state.ResetReach)
	if err := state.WithMailboxLock(t.Context(), dir, "api", func() error {
		return state.WriteAtomic(filepath.Join(t.TempDir(), "written"), []byte("x"))
	}); err != nil {
		t.Fatal(err)
	}
	written := boottime.Now()
	time.Sleep(5 * time.Millisecond)
	observeShell(Call{Command: &Command{Name: "send"}}, nil)
	notes := receipt.TakeShell(dir, "api", epoch)
	if len(notes) != 1 || !notes[0].OK || notes[0].Boot > written {
		t.Fatalf("a write done by %d left %+v", written, notes)
	}
}
