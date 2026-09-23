package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// The parts of the shim a scenario about grouped arrivals needs: a session
// that reads its mail the way the grouped-inbox contract describes, and a
// sender that can put two letters in one collection window and a third
// outside it.

// laterSendDelay separates the third letter from the first two. It is longer
// than the collection window by a wide margin, and longer than the server's
// retry interval too, so that a replay of announced-but-unread mail — which
// the retry interval gates — would show before the third letter arrives.
const laterSendDelay = 3 * time.Second

// readEach is what a session does with a delivery under shimReadEach: an
// overview that must consume nothing, then one member at a time. The last
// member of a group is deferred to the next delivery, so between the two the
// mailbox holds mail that was announced and stays unread — the only state in
// which "announced mail is never announced again" can be contradicted.
func (s *shimSession) readEach(notice mailboxNotice) (string, error) {
	if err := insideACase(); err != nil {
		return "", err
	}
	var summary []string
	if ids, err := s.peekOverview(); err != nil {
		s.recordRead("peek\t0\t\trefused\t" + firstLine(err.Error()))
		summary = append(summary, "peek refused")
	} else {
		s.recordRead(fmt.Sprintf("peek\t%d\t%s\tok\t", len(ids), strings.Join(ids, ",")))
		summary = append(summary, fmt.Sprintf("peek %d", len(ids)))
	}

	s.turn.mu.Lock()
	order := append([]string(nil), s.turn.deferred...)
	s.turn.deferred = nil
	s.turn.mu.Unlock()
	members := make([]string, 0, len(notice.Members))
	for _, member := range notice.Members {
		members = append(members, member.ID)
	}
	// A single-member delivery is read whole; only a group leaves its last
	// member for the next one.
	var deferred string
	if len(members) > 1 {
		deferred, members = members[len(members)-1], members[:len(members)-1]
	}
	order = append(order, members...)
	for _, id := range order {
		count, err := s.readOne(id)
		if err != nil {
			s.recordRead(fmt.Sprintf("message\t%s\trefused\t%s", id, firstLine(err.Error())))
			summary = append(summary, id+" refused")
			continue
		}
		s.recordRead(fmt.Sprintf("message\t%s\tok\t%d", id, count))
		summary = append(summary, id+" ok")
	}
	if deferred != "" {
		s.turn.mu.Lock()
		s.turn.deferred = append(s.turn.deferred, deferred)
		s.turn.mu.Unlock()
		s.recordRead("deferred\t" + deferred + "\t\t")
	}
	return "read-each: " + strings.Join(summary, ", "), nil
}

// peekOverview runs the overview and answers the ids it listed.
func (s *shimSession) peekOverview() ([]string, error) {
	out, err := s.inboxCall("--peek", "--json")
	if err != nil {
		return nil, err
	}
	var model struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(out, &model); err != nil {
		return nil, fmt.Errorf("unreadable overview: %w", err)
	}
	ids := make([]string, 0, len(model.Messages))
	for _, message := range model.Messages {
		ids = append(ids, message.ID)
	}
	return ids, nil
}

// readOne reads a single message by id and answers how many messages the
// call returned — one is the contract, and a number is recorded rather than a
// verdict so that the scenario decides.
func (s *shimSession) readOne(id string) (int, error) {
	out, err := s.inboxCall("--message", id, "--json")
	if err != nil {
		return 0, err
	}
	var model struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(out, &model); err != nil {
		return 0, fmt.Errorf("unreadable read: %w", err)
	}
	return len(model.Messages), nil
}

// inboxCall runs the built rewake's inbox command as the session and appends
// what came back to the mailbox record. A refusal carries the command's own
// words, which is what a scenario reports when a read did not happen.
func (s *shimSession) inboxCall(args ...string) ([]byte, error) {
	command := exec.Command("rewake", append([]string{"inbox"}, args...)...)
	command.Env = os.Environ()
	out, err := command.Output()
	if target := os.Getenv(shimMailboxFile); target != "" {
		if file, openErr := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); openErr == nil {
			_, _ = file.WriteString(strings.TrimSpace(string(out)) + "\n---\n")
			_ = file.Close()
		}
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return nil, errors.New(strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, err
	}
	return out, nil
}

// recordGroup writes one delivery down as a group: which turn carried it, how
// many members it named, which, and the notice text as it arrived.
func (s *shimSession) recordGroup(turn string, notice mailboxNotice) {
	target := os.Getenv(shimGroupsFile)
	if target == "" {
		return
	}
	ids := make([]string, 0, len(notice.Members))
	for _, member := range notice.Members {
		ids = append(ids, member.ID)
	}
	// Five fields, the last two of which this column leaves as they are: the
	// count is len(members) here, and the announcement carries no id of its
	// own. The other column fills them the other way round.
	appendLine(target, fmt.Sprintf("%s\t%d\t%s\t%s\t%s", turn, len(ids), strings.Join(ids, ","), strconv.Quote(notice.Notice), ""))
}

// recordRead appends one inbox call and its outcome.
func (s *shimSession) recordRead(line string) {
	if target := os.Getenv(shimReadsFile); target != "" {
		appendLine(target, line)
	}
}

func appendLine(target, line string) {
	file, err := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = fmt.Fprintln(file, line)
}

// rewakeCommand prepares the built rewake with this session's environment.
// Every call the shim makes goes through here, so the case's own rewake is the
// only one that can be reached.
func rewakeCommand(args ...string) *exec.Cmd {
	command := exec.Command("rewake", args...)
	command.Env = os.Environ()
	return command
}

// unmarshalJSON is json.Unmarshal under a name this file can share with the
// mid-turn half without importing encoding/json twice over.
func unmarshalJSON(raw []byte, into any) error { return json.Unmarshal(raw, into) }

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

// awaitRecipientReady waits for whatever readiness this column has. One column
// reports an accepted conversation in the room's telemetry; the other has no
// telemetry at all and says it is listening by creating a file. Asked for
// neither, the sender does not wait.
func awaitRecipientReady() bool {
	if mark := os.Getenv(shimWaitForFile); mark != "" {
		return awaitFile(mark)
	}
	if name := os.Getenv(shimSendWhenReady); name != "" {
		return awaitReady(name)
	}
	return true
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

// awaitReady polls the session's own view of the room until the named session
// has an accepted conversation. Bounded: a recipient that never gets there is
// the scenario's finding, not the sender's to wait on for ever.
func awaitReady(name string) bool {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		out, err := exec.Command("rewake", "list", "--json").Output()
		if err == nil {
			var current listing
			if json.Unmarshal(out, &current) == nil {
				if _, selection, _, found := current.find(name); found && selection == "ready" {
					return true
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}
