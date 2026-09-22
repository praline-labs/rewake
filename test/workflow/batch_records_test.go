package workflow

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

// What a session under shimReadEach writes down, read back by a scenario. The
// files are the session's own record: the scenario never looks at the mailbox
// itself, because the claim is about what the session was told and did.

// groupDelivery is one delivery as the session recorded it.
//
// Members is empty on a column whose notice does not name them; Count is what
// the notice announced, and Announced is the id it carried. A scenario that
// needs the members asks whether they are there rather than assuming.
type groupDelivery struct {
	Turn      string
	Count     int
	Members   []string
	Notice    string
	Announced string
}

// groupDeliveries are the deliveries in the order they arrived. A line that
// cannot be parsed is an error, not an empty delivery: a record the scenario
// cannot read is a scenario that cannot judge.
func (s *codexSession) groupDeliveries() ([]groupDelivery, error) {
	raw, err := os.ReadFile(s.groups)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var deliveries []groupDelivery
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) != 5 {
			return nil, errUnreadableRecord(s.groups, line)
		}
		count, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, errUnreadableRecord(s.groups, line)
		}
		var members []string
		if fields[2] != "" {
			members = strings.Split(fields[2], ",")
			if count != len(members) {
				return nil, errUnreadableRecord(s.groups, line)
			}
		}
		notice, err := strconv.Unquote(fields[3])
		if err != nil {
			return nil, errUnreadableRecord(s.groups, line)
		}
		deliveries = append(deliveries, groupDelivery{Turn: fields[0], Count: count, Members: members, Notice: notice, Announced: fields[4]})
	}
	return deliveries, nil
}

// readRecord is one inbox call the session made: a peek with the ids it
// listed, a read of one message with how many it returned, or a deferral.
type readRecord struct {
	Kind   string // peek, message, deferred
	ID     string // for message and deferred
	IDs    []string
	OK     bool
	Detail string
}

func (s *codexSession) readRecords() ([]readRecord, error) {
	raw, err := os.ReadFile(s.reads)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []readRecord
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 5)
		if len(fields) < 4 {
			return nil, errUnreadableRecord(s.reads, line)
		}
		record := readRecord{Kind: fields[0]}
		switch fields[0] {
		case "peek":
			if fields[2] != "" {
				record.IDs = strings.Split(fields[2], ",")
			}
			record.OK = fields[3] == "ok"
			if len(fields) == 5 {
				record.Detail = fields[4]
			}
		case "message":
			record.ID, record.OK, record.Detail = fields[1], fields[2] == "ok", fields[3]
		case "deferred":
			record.ID = fields[1]
		default:
			return nil, errUnreadableRecord(s.reads, line)
		}
		records = append(records, record)
	}
	return records, nil
}

// overviews are the ids each overview listed, in the order the session took
// them. One overview per delivery on a column that reads that way, which is
// what makes "what this delivery announced" answerable where the notice does
// not say.
func (s *codexSession) overviews() ([][]string, error) {
	records, err := s.readRecords()
	if err != nil {
		return nil, err
	}
	var taken [][]string
	for _, entry := range records {
		if entry.Kind == "peek" && entry.OK {
			taken = append(taken, entry.IDs)
		}
	}
	return taken, nil
}

// peekedMessage is one row of an overview the session took, as the session's
// own machine-form call returned it.
type peekedMessage struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Preview   string    `json:"preview"`
}

// peekedMessages are every row of every overview the session took, in order.
// Overviews are told apart from reads by their shape: a read carries bodies,
// an overview carries previews.
func (s *codexSession) peekedMessages() []peekedMessage {
	var all []peekedMessage
	for _, chunk := range strings.Split(s.mailboxRead(), "\n---\n") {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		var model struct {
			Messages []peekedMessage `json:"messages"`
		}
		if json.Unmarshal([]byte(chunk), &model) != nil {
			continue
		}
		for _, message := range model.Messages {
			if message.Preview != "" {
				all = append(all, message)
			}
		}
	}
	return all
}

// peekedCarrying finds the overview row whose preview carries a marker: that
// is how a scenario learns the id and the time of a letter it knows by text.
func (s *codexSession) peekedCarrying(marker string) (peekedMessage, bool) {
	for _, message := range s.peekedMessages() {
		if strings.Contains(message.Preview, marker) {
			return message, true
		}
	}
	return peekedMessage{}, false
}

type recordError struct{ file, line string }

func (e recordError) Error() string {
	return "unreadable record in " + e.file + ": " + strconv.Quote(e.line)
}

func errUnreadableRecord(file, line string) error { return recordError{file: file, line: line} }
