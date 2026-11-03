package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestClaudeAdopt runs the real Claude Code adapter's launch, handover and
// adoption against a stand-in that listens on the messaging socket as Claude
// Code does. On November 3, 2026 every real Claude Code run failed to be
// adopted after a switch: the adopted image planned the launch again, which
// refused the socket the running harness listens on as "a live socket", and
// the wrapper exited and took main and the room with it (docs/traps.md). Here
// main's wrapper re-executes on SIGUSR2 and must go on as the same run: its
// pid and harness kept, a letter after the exec reaching the stand-in's
// socket as one before it did.
func TestClaudeAdopt(t *testing.T) {
	binary := enterScenario(t, "claude-adopt")
	c := Start(t, Spec{Name: "claude-adopt", Harness: "claude", Observations: claudeAdoptObservations, Deadline: 180 * time.Second})
	iso := Isolate(t, c, binary)
	installShim(t, c, iso)
	if err := installClaudeStandIn(iso); err != nil {
		t.Fatal(err)
	}
	lead := startHarnessSession(t, c, iso, "claude", "lead", "--main")
	asks := newRequests(iso, "peer")
	peer := startHarnessSession(t, c, iso, fixtureColumn.harness, "peer", "--general", asks.env(), shimInboxJSON+"=1")
	defer stopSession(t, c, peer)
	defer stopSession(t, c, lead)
	if !waitFor(c, 20*time.Second, func() bool {
		return exists(lead.ready) && exists(peer.ready)
	}) {
		c.Contradicted(obsClaudeAdoptServed, "the sessions never became ready: lead %v, peer %v", exists(lead.ready), exists(peer.ready))
		return
	}
	// A notice previews only the first unread letter, so a letter is seen
	// to reach the socket by the notice it adds.
	notices := func() int {
		raw, _ := os.ReadFile(lead.delivered)
		return strings.Count(string(raw), "\n")
	}
	notify := func(text string) bool {
		seen := notices()
		code, out, _ := asks.ask(c, "send", lead.name, text, "--notify")
		if code != 0 {
			c.Note(fmt.Sprintf("send %q answered %d: %s", text, code, out))
		}
		return waitFor(c, 20*time.Second, func() bool { return notices() > seen })
	}
	if !notify("claude-adopt: before the exec") {
		c.Contradicted(obsClaudeAdoptServed, "a letter before the exec never reached the stand-in's socket")
		return
	}

	before := reexecRecordOf(iso, lead.name)
	mark := filepath.Join(filepath.Dir(filepath.Dir(before.path)), "run", lead.name+"."+before.epoch(), "reexec")
	// A notice Claude Code accepted says nothing back, and the lane keeps it
	// for its late word a minute (mail.LateWordWindow): the exec is refused
	// until then, and the signal is sent again.
	var outcome string
	waitFor(c, 90*time.Second, func() bool {
		outcome = signalReexec(c, before.ServicePID, mark)
		if strings.HasPrefix(outcome, "refused") {
			time.Sleep(time.Second)
			return false
		}
		return true
	})
	after := reexecRecordOf(iso, lead.name)
	switch {
	case outcome != "adopted 1" || !replaced(before.ServicePID):
		c.Contradicted(obsClaudeAdoptKept, "the exec answered %q; the wrapper replaced %v, alive %v; its stderr: %s", outcome, replaced(before.ServicePID), alive(before.ServicePID), stderrText(iso, "lead"))
	case after.epoch() != before.epoch() || after.HarnessPID != before.HarnessPID || !alive(before.HarnessPID):
		c.Contradicted(obsClaudeAdoptKept, "the record moved or the harness went: %+v, was %+v", after, before)
	default:
		c.Observed(obsClaudeAdoptKept, fmt.Sprintf("wrapper %d and harness %d kept, epoch kept", before.ServicePID, before.HarnessPID))
	}
	if notify("claude-adopt: after the exec") {
		c.Observed(obsClaudeAdoptServed, "letters before and after the exec reached the messaging socket")
	} else {
		c.Contradicted(obsClaudeAdoptServed, "the letter after the exec never reached the stand-in's socket")
	}
}

// stderrText is what a session's wrapper said on its standard error.
func stderrText(iso *Isolation, label string) string {
	raw, _ := os.ReadFile(filepath.Join(iso.Home, label+".stderr"))
	return strings.TrimSpace(string(raw))
}

const (
	obsClaudeAdoptKept   = "a Claude Code main's wrapper re-executes and adopts its run, keeping its pid, epoch and harness"
	obsClaudeAdoptServed = "the adopted Claude Code run is delivered a letter on its messaging socket"
)

var claudeAdoptObservations = []string{obsClaudeAdoptKept, obsClaudeAdoptServed}
