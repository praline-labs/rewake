package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// How a session of the suite sends: the sender in a delivery scenario is a
// session with its own rewake, not the test process.

// laterSendDelay separates the third letter from the first two. It is longer
// than the collection window by a wide margin, and longer than the server's
// retry interval too, so that a replay of announced-but-unread mail — which
// the retry interval gates — would show before the third letter arrives.
const laterSendDelay = 3 * time.Second

// noteSpacing separates the heads-ups from the later letter and from each
// other: far outside the first collection, and well inside suiteQuiet, the
// quiet a heads-up waits in for company in the suite's build; the three span
// less than suiteCap.
const noteSpacing = 600 * time.Millisecond

// sendAsAsked is the sender's part: one letter, or several at the same moment
// and one more after a delay. The sender is a session too — it sends with its
// own rewake, the way a session does, not the way a test would.
func sendAsAsked() int {
	target := os.Getenv(shimSendTo)
	if target == "" {
		return 0
	}
	if err := insideACase(); err != nil {
		fmt.Fprintf(os.Stderr, "shim: %v\n", err)
		return 1
	}
	if !awaitRecipientReady() {
		// Not an exit: the letters still go, and the scenario will see them
		// folded into one group and say so. Leaving would end the session and
		// turn a readiness problem into a missing sender.
		fmt.Fprintln(os.Stderr, "shim: the recipient never became ready; sending anyway")
	}
	texts := []string{os.Getenv(shimSendText)}
	if list := os.Getenv(shimSendTexts); list != "" {
		texts = strings.Split(list, "|")
	}
	// All started before any is waited for: `rewake send` waits for its
	// delivery, and two letters sent one after the other would land in two
	// windows.
	started := time.Now()
	var running []*exec.Cmd
	var outputs []*strings.Builder
	for _, text := range texts {
		send := exec.Command("rewake", "send", target, text)
		send.Env = os.Environ()
		output := &strings.Builder{}
		send.Stdout, send.Stderr = output, output
		if err := send.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "shim: sending to %s: %v\n", target, err)
			continue
		}
		running = append(running, send)
		outputs = append(outputs, output)
	}
	for index, send := range running {
		err := send.Wait()
		if err != nil {
			fmt.Fprintf(os.Stderr, "shim: sending to %s: %v\n", target, err)
		}
		// What the sender was told, and the exit code that told it: a letter
		// held on the way is accepted and not delivered, and only both say so.
		recordSend(fmt.Sprintf("letter-%d", index+1), fmt.Sprintf("exit=%d", send.ProcessState.ExitCode()), firstLine(outputs[index].String()))
	}
	// A second letter timed against the recipient's own state, for the
	// mid-turn scenario. It runs after the first letters have been accepted,
	// because a turn has to be open before anything can be steered into it.
	sendSecond()
	// In the background: one column reports its own state only once this
	// returns, and the heads-ups take seconds to leave.
	go sendNotes(target, started)
	if later := os.Getenv(shimSendLaterText); later != "" {
		// Measured from the moment the first letters left, not from when
		// their deliveries were confirmed: a wider window would delay the
		// confirmations too, and the third letter is meant to test the window.
		time.Sleep(time.Until(started.Add(laterSendDelay)))
		send := exec.Command("rewake", "send", target, later)
		send.Env = os.Environ()
		if out, err := send.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "shim: sending to %s: %v: %s\n", target, err, out)
		}
	}
	return 0
}

// sendNotes sends the heads-ups, each at its own moment from when the first
// letters left, without waiting for the one before: a send waits for its
// delivery, and a heads-up's delivery waits for company.
func sendNotes(target string, started time.Time) {
	list := os.Getenv(shimSendNotes)
	if list == "" {
		return
	}
	var running sync.WaitGroup
	for index, text := range strings.Split(list, "|") {
		time.Sleep(time.Until(started.Add(laterSendDelay + time.Duration(index+1)*noteSpacing)))
		running.Add(1)
		go func() {
			defer running.Done()
			send := exec.Command("rewake", "send", target, text, "--notify")
			send.Env = os.Environ()
			out, err := send.CombinedOutput()
			code := -1
			if send.ProcessState != nil {
				code = send.ProcessState.ExitCode()
			} else if err != nil {
				out = []byte(err.Error())
			}
			recordSend(fmt.Sprintf("note-%d", index+1), fmt.Sprintf("exit=%d", code), firstLine(string(out)))
		}()
	}
	running.Wait()
}

// awaitRecipientReady waits for whatever readiness this column has. One column
// reports an accepted conversation in the room's telemetry; the other has no
// telemetry at all and says it is listening by creating a file. Asked for
// neither, the sender does not wait.
func awaitRecipientReady() bool {
	if mark := os.Getenv(shimWaitForFile); mark != "" {
		return awaitFile(mark)
	}
	return true
}

// sendSecond is the sender's part of a mid-turn scenario: wait for the
// recipient to reach a named state, then send one more letter.
//
// The state is read from the recipient's own telemetry through this session's
// rewake, not from a sleep: "working" is the thread reporting an active status,
// which it does when its turn begins. That is the same evidence the live probe
// of September 21, 2026 used.
func sendSecond() {
	text := os.Getenv(shimSendSecondText)
	target := os.Getenv(shimSendTo)
	if text == "" || target == "" {
		return
	}
	want, state := os.Getenv(shimSendWhenWorking), "working"
	if want == "" {
		want, state = os.Getenv(shimSendWhenIdle), "idle"
	}
	if want == "" {
		return
	}
	seen, reached := awaitActivity(want, state)
	if !reached {
		// Not an exit: the letter still goes, and the scenario reads the
		// recipient's record to see that it did not arrive mid-turn. Leaving
		// here would turn a finding about delivery into a missing sender.
		fmt.Fprintf(os.Stderr, "shim: %s never reported %s; sending anyway\n", want, state)
	}
	// What the sender saw before it sent, recorded whether or not it was what
	// it waited for: a letter that went out at the wrong moment and one that
	// went out at the right one look identical afterwards.
	recordSend("second-wait", state, "last seen "+seen)
	send := rewakeCommand("send", target, text)
	out, err := send.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "shim: sending to %s: %v: %s\n", target, err, out)
		recordSend("second", "refused", firstLine(strings.TrimSpace(string(out))))
		return
	}
	recordSend("second", "accepted", firstLine(strings.TrimSpace(string(out))))
}

// recordSend writes down what became of one letter this session sent. A
// refusal is a result like any other, and a scenario that could not see one
// would read a letter that was never delivered as a letter still on its way.
func recordSend(which, outcome, detail string) {
	if target := os.Getenv(shimSendsFile); target != "" {
		appendLine(target, fmt.Sprintf("%s\t%s\t%s", which, outcome, detail))
	}
}

// rewakeCommand prepares the built rewake with this session's environment.
// Every call the shim makes goes through here, so the case's own rewake is the
// only one that can be reached.
func rewakeCommand(args ...string) *exec.Cmd {
	command := exec.Command("rewake", args...)
	command.Env = os.Environ()
	return command
}

// awaitFile waits for a file to appear. Bounded like every other wait here: a
// recipient that never listens is the scenario's finding, not the sender's to
// wait on for ever.
func awaitFile(path string) bool {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// awaitActivity polls this session's own view of the room until the named
// session reports the state asked for. Bounded: a recipient that never gets
// there is the scenario's finding, not the sender's to wait on for ever.
//
// "idle" has to be seen *after* working, or the control that must deliver
// after the turn would send while the recipient had not started yet — which
// is not "after the turn ends" but "before it begins", and the two look the
// same in a snapshot.
func awaitActivity(name, state string) (string, bool) {
	deadline := time.Now().Add(holdWindow + 10*time.Second)
	worked := state != "idle"
	seen := "nothing"
	for time.Now().Before(deadline) {
		if current, ok := activityOf(name); ok {
			seen = current
			if current == "working" {
				worked = true
			}
			if worked && current == state {
				return seen, true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return seen, false
}

// activityOf answers the activity this session's rewake reports for another,
// and whether it could be read at all. A listing that cannot be read is not an
// absent activity: the caller keeps waiting rather than concluding.
func activityOf(name string) (string, bool) {
	out, err := rewakeCommand("list", "--json").Output()
	if err != nil {
		return "", false
	}
	var current listing
	if json.Unmarshal(out, &current) != nil {
		return "", false
	}
	activity, _, _, found := current.find(name)
	if !found {
		return "", false
	}
	return activity, true
}
