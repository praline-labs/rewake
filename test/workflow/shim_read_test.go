package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// How a session of the suite reads its mail: whole, or an overview first and
// one member at a time.

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

// recordRead appends one inbox call and its outcome.
func (s *shimSession) recordRead(line string) {
	if target := os.Getenv(shimReadsFile); target != "" {
		appendLine(target, line)
	}
}

// readMailbox runs the built rewake, as the session, and returns what it read.
//
// The guard comes first, and it is not a precaution against a mistake that has
// not happened: a turn accepted inside a test starts this work in a goroutine,
// and by the time the goroutine runs the test may have restored the ambient
// environment — the one belonging to a session somebody is working in. The
// shim refuses to run anything outside its own case, so leaving the isolation
// is impossible rather than unlikely.
func (s *shimSession) readMailbox() (string, error) {
	if err := insideACase(); err != nil {
		return "", err
	}
	if os.Getenv(shimReadFails) != "" {
		return "", errors.New("the mailbox read was made to fail")
	}
	waitAtReadGate()
	arguments := []string{"inbox"}
	if os.Getenv(shimInboxJSON) != "" {
		arguments = append(arguments, "--json")
	}
	command := exec.Command("rewake", arguments...)
	command.Env = os.Environ()
	out, err := command.Output()
	if err != nil {
		return "", err
	}
	read := strings.TrimSpace(string(out))
	if target := os.Getenv(shimMailboxFile); target != "" {
		// Appended, not replaced: a session is delivered more than one thing
		// — availability notices arrive before any task — and a scenario
		// looking for its own message must be able to find it among them.
		if file, err := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			_, _ = file.WriteString(read + "\n---\n")
			_ = file.Close()
		}
	}
	if read == "" {
		return "", errors.New("the mailbox was empty when the session read it")
	}
	// The whole of what was read, not its first line: an inbox reply opens
	// with a header naming the sender, the kind and the time, and the message
	// itself comes after it. A report quoting only the header would say
	// nothing about which message it answers.
	return "read: " + read, nil
}
