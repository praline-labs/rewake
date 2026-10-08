//go:build rewakefixture

package toolrig

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness/fixture"
	"github.com/praline-labs/rewake/internal/receipt"
)

// The gaps S7 closes on the neutral rig (docs/v2/stage3-tests-tcl.md): what
// no test of the old rig said, because its transport was one harness's.

var (
	runIDs   = regexp.MustCompile(`[0-9a-f]{12,}`)
	runTimes = regexp.MustCompile(`\d\d:\d\d:\d\d`)
)

// normalized is an answer without what differs between two runs: ids and
// times.
func normalized(text string) string {
	return runTimes.ReplaceAllString(runIDs.ReplaceAllString(text, "*"), "*")
}

// T1: the transport keeps nothing a later call needs. The same calls of one
// turn answer the same whether the harness's process runs through them all
// or is restarted between each two: what a call did lives in the receipts.
func TestARestartedTransportChangesNoAnswer(t *testing.T) {
	t.Parallel()
	turn := [][]string{
		{"inbox"},
		{"send", "--notify", "--wait", "0", "web", faultHeadsUp},
		{"send", "--notify", "--wait", "0", "web", faultHeadsUp},
		{"inbox"},
		{"pending", "the work goes on"},
		{"pending", "the work goes on"},
	}
	run := func(restart bool) []string {
		r := newRig(t)
		r.start()
		r.letter("the letter of the restart test")
		r.nextTurn()
		var answers []string
		for i, words := range turn {
			if restart && i > 0 {
				r.start()
			}
			c := r.call(words...)
			if c.ended {
				t.Fatalf("%v: the program ended", words)
			}
			_ = r.complete(c, !c.result.IsError)
			answers = append(answers, normalized(c.result.text()))
		}
		if n := headsUps(r); n != 1 {
			t.Fatalf("restarted %v: web holds %d heads-ups", restart, n)
		}
		return answers
	}
	through, restarted := run(false), run(true)
	for i := range turn {
		if through[i] != restarted[i] {
			t.Fatalf("%v answered\n%s\nwith one process, and\n%s\nrestarted", turn[i], through[i], restarted[i])
		}
	}
	if !strings.Contains(through[0], "the letter of the restart test") || !strings.Contains(through[4], "marked pending") {
		t.Fatalf("the turn's answers: %q", through)
	}
}

// T4: an answer that proves the call's effect absent leaves the same words to
// the shell, which runs them once: a child ended before it began any
// operation published nothing, and the shell's send is the one heads-up.
func TestTheShellRunsTheSameWordsOnTheProofOfAbsence(t *testing.T) {
	t.Parallel()
	for _, words := range [][]string{
		{"send", "--notify", "--wait", "0", "web", faultHeadsUp},
		{"inbox"},
	} {
		t.Run(words[0], func(t *testing.T) {
			t.Parallel()
			r := newRig(t)
			r.fault = "child:crash=1"
			r.start()
			id := r.letter(faultLetter)
			r.nextTurn()
			c := r.call(words...)
			text := c.result.text()
			if c.ended || !c.result.IsError || !strings.Contains(text, "here or in the shell") || retryWords.MatchString(text) {
				t.Fatalf("a child ended before its first step answered: %s", text)
			}
			_ = r.complete(c, false)
			if headsUps(r) != 0 || !r.unread(id) {
				t.Fatalf("the absent call left %d heads-ups, unread %v", headsUps(r), r.unread(id))
			}
			r.fault = ""
			// A send that waits for nothing exits 3, its result still to come.
			out, _ := r.shell(words...)
			switch words[0] {
			case "send":
				if n := headsUps(r); n != 1 || !strings.Contains(out, "pending for web") {
					t.Fatalf("the shell's send: %d heads-ups: %s", n, out)
				}
			case "inbox":
				if r.unread(id) || !strings.Contains(out, faultLetter) {
					t.Fatalf("the shell's read: unread %v: %s", r.unread(id), out)
				}
			}
		})
	}
}

// T11: one set of words runs once through the tool and once through the
// shell under one receipt: whichever channel began the operation, the other
// finishes or finds it, and its effect is one.
func TestOneSetOfWordsThroughTheToolAndTheShellIsOneOperation(t *testing.T) {
	t.Parallel()
	words := []string{"send", "--notify", "--wait", "0", "web", faultHeadsUp}
	t.Run("the tool first", func(t *testing.T) {
		t.Parallel()
		r := newRig(t)
		r.start()
		r.nextTurn()
		r.plan(&wrapperPlan{killAt: "answer", kill: r.killProgram})
		c := r.call(words...)
		if !c.ended || headsUps(r) != 1 {
			t.Fatalf("the tool's send: ended %v, %d heads-ups", c.ended, headsUps(r))
		}
		token := boundToken(r, c)
		out, _ := r.shell("retry", token)
		if !strings.Contains(out, "ran earlier (receipt "+token+")") || !strings.Contains(out, "pending for web") || headsUps(r) != 1 {
			t.Fatalf("the shell's retry: %d heads-ups: %s", headsUps(r), out)
		}
	})
	t.Run("the shell first", func(t *testing.T) {
		t.Parallel()
		// The shell's send ended before its last durable step: published,
		// its operation left open.
		probe := newRig(t)
		probe.nextTurn()
		logPath := filepath.Join(probe.root, "shell.log")
		probe.fault = "other:log=" + logPath
		if out, _ := probe.shell(words...); !strings.Contains(out, "pending for web") {
			t.Fatalf("the clean send: %s", out)
		}
		last := len(durable(parseLog(t, logPath, probe.root), "other"))
		r := newRig(t)
		r.nextTurn()
		r.fault = "other:crash=" + strconv.Itoa(last)
		_, _ = r.shell(words...)
		r.fault = ""
		if n := headsUps(r); n != 1 {
			t.Fatalf("the cut shell send published %d heads-ups", n)
		}
		// The tool's same words find the operation, and its retry ends it.
		c := r.call(words...)
		again := c.result.text()
		if found := retryWords.FindStringSubmatch(again); found != nil {
			c = r.call("retry", found[1])
		}
		if c.ended || !strings.Contains(c.result.text(), "pending for web") || headsUps(r) != 1 {
			t.Fatalf("the tool after the shell: %d heads-ups: %q, then %q", headsUps(r), again, c.result.text())
		}
	})
}

// boundToken is the receipt c's binding names.
func boundToken(r *rig, c toolCall) string {
	r.t.Helper()
	token, err := receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(fixture.Transport, thread, c.id))
	if err != nil {
		r.t.Fatal(err)
	}
	return token
}

// E4: a read's completion is one durable fact the tool and the shell share. A
// letter the tool read is gone for the shell; a letter the shell read while a
// tool's answer showing it was on its way is gone for the tool, and the tool's
// late acknowledgment finds it read and brings nothing back.
func TestAReadIsOneFactAcrossTheToolAndTheShell(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	first := r.letter("the letter the tool reads")
	r.nextTurn()
	c := r.call("inbox")
	if err := r.complete(c, true); err != nil || r.unread(first) {
		t.Fatalf("the tool's read: %v, unread %v", err, r.unread(first))
	}
	if out, _ := r.shell("inbox"); strings.Contains(out, "the letter the tool reads") {
		t.Fatalf("the shell shows what the tool read: %s", out)
	}
	// Each read in a turn of its own: the same words in one turn are one read.
	second := r.letter("the letter the shell reads")
	r.nextTurn()
	shown := r.call("inbox")
	if !strings.Contains(shown.result.text(), "the letter the shell reads") {
		t.Fatalf("the tool's answer: %+v", shown.result)
	}
	if out, _ := r.shell("inbox"); !strings.Contains(out, "the letter the shell reads") || r.unread(second) {
		t.Fatalf("the shell's read: unread %v: %s", r.unread(second), out)
	}
	_ = r.complete(shown, true)
	if r.unread(first) || r.unread(second) {
		t.Fatal("a late acknowledgment brought a letter back")
	}
	r.nextTurn()
	again := r.call("inbox")
	if text := again.result.text(); strings.Contains(text, "the letter the tool reads") || strings.Contains(text, "the letter the shell reads") {
		t.Fatalf("the tool shows a letter read: %s", text)
	}
	if out, _ := r.shell("inbox"); strings.Contains(out, "the letter") {
		t.Fatalf("the shell shows a letter read: %s", out)
	}
}
